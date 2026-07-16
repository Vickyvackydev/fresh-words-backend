package services

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fresh-words-backend/models"
	"fresh-words-backend/services/extractor"
	"fresh-words-backend/services/normalizer"
	"fresh-words-backend/services/reconstructor"
	"fresh-words-backend/services/typography"
	"github.com/google/uuid"
)

type ValidationIssue struct {
	DayOfYear int    `json:"day_of_year"`
	DateText  string `json:"date_text"`
	Severity  string `json:"severity"` // "error" or "warning"
	Message   string `json:"message"`
}

type ParserReport struct {
	TotalParsed int                 `json:"total_parsed"`
	IsValid     bool                `json:"is_valid"`
	Issues      []ValidationIssue   `json:"issues"`
	Devotionals []models.Devotional `json:"-"`
}

// ParseDocument reads the file and extracts a list of devotionals using the new Document Reconstruction Engine.
func ParseDocument(filePath string, category string, year int, packageID uuid.UUID) (*ParserReport, error) {
	filePathLower := strings.ToLower(filePath)
	var physicalDoc *extractor.Document
	var err error

	// Step 1: Physical Extraction
	if strings.HasSuffix(filePathLower, ".pdf") {
		physicalDoc, err = extractor.ReadPdf(filePath)
	} else if strings.HasSuffix(filePathLower, ".docx") || strings.HasSuffix(filePathLower, ".doc") {
		// Attempt to parse .doc as .docx (sometimes users just rename the extension)
		physicalDoc, err = extractor.ReadDocx(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read DOCX (if this is a legacy .doc file, please convert it to .docx first): %w", err)
		}
	} else {
		return nil, fmt.Errorf("unsupported file format: only PDF and DOCX files are allowed")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to extract document: %w", err)
	}

	// Step 2 & 3: Layout Reconstruction (Paragraph merging, Headers/Footers)
	reconstructedDoc, err := reconstructor.Reconstruct(physicalDoc)
	if err != nil {
		return nil, fmt.Errorf("reconstruction failed: %w", err)
	}

	// Step 4: Typography & Semantic Detection
	semanticDoc := typography.Detect(reconstructedDoc)

	// Step 5: Normalization
	normalizedDoc := normalizer.Normalize(semanticDoc)

	// Step 7, 8, 9: Parse the Document Tree into Devotional objects
	report := extractDevotionals(normalizedDoc, category, year, packageID)

	return report, nil
}

// extractDevotionals iterates through the logical blocks to build devotionals based on structure.
func extractDevotionals(doc *typography.SemanticDocument, category string, year int, packageID uuid.UUID) *ParserReport {
	report := &ParserReport{
		IsValid: true,
	}

	var currentDevo *models.Devotional
	var currentSection string
	var dayCounter int = 1

	for i := 0; i < len(doc.Blocks); i++ {
		block := doc.Blocks[i]
		text := strings.TrimSpace(block.Text)

		if text == "" {
			continue
		}

		isDate := isDateOrDay(text)
		log.Printf("[PARSER] Evaluating Block - Type: %v, Length: %d, IsDate: %v | Text: %q\n", block.Type, len(text), isDate, text)

		// E.g., "JANUARY 4" or "Day 4"
		// If it looks like a date and it's short, treat it as the start of a Devotional.
		// We relax the TypeHeading requirement because some PDFs have dates formatted identically to body text.
		if isDate && len(text) < 150 {
			log.Printf("[PARSER] --> NEW DEVOTIONAL BOUNDARY FOUND: %s\n", text)
			// Save the previous devotional
			if currentDevo != nil {
				report.Devotionals = append(report.Devotionals, *currentDevo)
			}

			targetDay := dayCounter
			parsedDay, err := parseDayFromDateString(text, year)
			if err == nil && parsedDay > 0 {
				targetDay = parsedDay
				dayCounter = parsedDay + 1
			} else {
				dayCounter++
			}

			currentDevo = &models.Devotional{
				ID:         uuid.New(),
				PackageID:  packageID,
				Category:   category,
				DefaultDay: targetDay,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			currentSection = "Date"
			continue
		}

		if currentDevo == nil {
			continue // skip preamble until the first date/devotional is found
		}

		// State machine based on Headings
		if block.Type == typography.TypeHeading {
			upperText := strings.ToUpper(text)

			// If it's a heading right after Date, it's ALWAYS the Title
			if currentSection == "Date" {
				ref, quote, found := extractScripture(text)
				if found {
					currentDevo.Title = quote
					currentDevo.ScriptureReference = ref
					currentSection = "ScriptureQuote"
				} else {
					currentDevo.Title = text
					currentSection = "Title"
				}
				continue
			}

			// Avoid matching titles containing the keywords by enforcing a length threshold
			isSectionHeader := false
			if len(text) < 35 {
				switch {
				case strings.Contains(upperText, "PRAYER") || strings.Contains(upperText, "PRAYER POINT"):
					currentSection = "Prayer"
					isSectionHeader = true
				case strings.Contains(upperText, "CONFESSION") || strings.Contains(upperText, "REFLECTION"):
					currentSection = "Reflection"
					isSectionHeader = true
				case strings.Contains(upperText, "FURTHER READING"):
					currentSection = "FurtherReading"
					isSectionHeader = true
				case strings.Contains(upperText, "MEMORY VERSE"):
					currentSection = "MemoryVerse"
					isSectionHeader = true
				case strings.Contains(upperText, "KEY POINT") || strings.Contains(upperText, "ACTION POINT"):
					currentSection = "ActionPoints"
					isSectionHeader = true
				case strings.Contains(upperText, "DECLARATION"):
					currentSection = "Declaration"
					isSectionHeader = true
				}
			}

			if isSectionHeader {
				continue
			}

			// If it's a heading block during Title state, check if it is a scripture reference
			if currentSection == "Title" {
				ref, quote, found := extractScripture(text)
				if found {
					currentDevo.ScriptureReference = ref
					if quote != "" {
						currentDevo.ScriptureQuote = quote
						currentSection = "Body"
					} else {
						currentSection = "ScriptureQuote"
					}
				} else {
					currentDevo.Title += " " + text
				}
				continue
			}

			// Fallback for unknown headings
			currentSection = "Body"
			if currentDevo.Body != "" {
				currentDevo.Body += "\n\n"
			}
			currentDevo.Body += text
			continue
		}

		// Paragraphs map to the active section
		switch currentSection {
		case "Title":
			// Extract both scripture quote and reference if they reside in the same block
			ref, quote, found := extractScripture(text)
			if found {
				currentDevo.ScriptureReference = ref
				if quote != "" {
					currentDevo.ScriptureQuote = quote
					currentSection = "Body"
				} else {
					currentSection = "ScriptureQuote"
				}
			} else {
				currentDevo.Body += text
				currentSection = "Body"
			}
		case "ScriptureQuote":
			currentDevo.ScriptureQuote += text
			currentSection = "Body"
		case "Prayer":
			if currentDevo.Prayer != "" {
				currentDevo.Prayer += "\n"
			}
			currentDevo.Prayer += text
		case "Reflection":
			if currentDevo.Reflection != "" {
				currentDevo.Reflection += "\n"
			}
			currentDevo.Reflection += text
		case "ActionPoints":
			if currentDevo.ActionPoints != "" {
				currentDevo.ActionPoints += "\n"
			}
			currentDevo.ActionPoints += text
		case "Body":
			if currentDevo.Body != "" {
				currentDevo.Body += "\n\n"
			}
			currentDevo.Body += text
		default:
			if currentDevo.Body != "" {
				currentDevo.Body += "\n\n"
			}
			currentDevo.Body += text
		}
	}

	// Append the very last devotional
	if currentDevo != nil {
		report.Devotionals = append(report.Devotionals, *currentDevo)
	}

	report.TotalParsed = len(report.Devotionals)

	if report.TotalParsed == 0 {
		report.IsValid = false
		report.Issues = append(report.Issues, ValidationIssue{
			Severity: "error",
			Message:  "No devotionals could be extracted. Check date headings.",
		})
	}

	return report
}

// Helpers
func isDateOrDay(text string) bool {
	upper := strings.ToUpper(strings.TrimSpace(text))
	// Match things like "JANUARY 4", "Jan 4", "DAY 4"
	re := regexp.MustCompile(`^(JANUARY|FEBRUARY|MARCH|APRIL|MAY|JUNE|JULY|AUGUST|SEPTEMBER|OCTOBER|NOVEMBER|DECEMBER|JAN|FEB|MAR|APR|JUN|JUL|AUG|SEP|OCT|NOV|DEC|DAY)\s+\d{1,2}`)
	return re.MatchString(upper)
}

func isScriptureReference(text string) bool {
	return strings.Contains(text, ":") || strings.Contains(text, " 1 ") || strings.Contains(text, " 2 ")
}

func extractScripture(text string) (ref string, quote string, found bool) {
	// Match book, chapter and verse (e.g. Luke 18:1, 1 Corinthians 13:4-8, Psalm 68:9)
	re := regexp.MustCompile(`\(?((?:[1-9]\s+)?[A-Za-z]+(?:\s+[A-Za-z]+)*\s+\d+:\d+(?:\s*[-–—]\s*\d+)?)\)?`)
	loc := re.FindStringSubmatchIndex(text)
	if loc == nil {
		// Fallback to match book and chapter only (e.g. "1 Chronicles 29-30")
		reNoVerse := regexp.MustCompile(`\(?((?:[1-9]\s+)?[A-Za-z]+(?:\s+[A-Za-z]+)*\s+\d+(?:\s*[-–—]\s*\d+)?)\)?`)
		loc = reNoVerse.FindStringSubmatchIndex(text)
		if loc == nil {
			return "", "", false
		}
	}

	ref = text[loc[2]:loc[3]]
	
	// Remove the matched reference from the text to get the quote
	rawQuote := text[:loc[0]] + text[loc[1]:]
	rawQuote = strings.TrimSpace(rawQuote)
	// Clean up quotes, brackets and trailing punctuation
	rawQuote = strings.Trim(rawQuote, `()[]{}""'';,. `)
	rawQuote = strings.TrimSpace(rawQuote)

	return ref, rawQuote, true
}

func parseDayFromDateString(text string, year int) (int, error) {
	upper := strings.ToUpper(strings.TrimSpace(text))
	
	if strings.HasPrefix(upper, "DAY") {
		var dayNum int
		_, err := fmt.Sscanf(upper, "DAY %d", &dayNum)
		if err == nil {
			return dayNum, nil
		}
	}

	months := map[string]time.Month{
		"JANUARY": time.January, "JAN": time.January,
		"FEBRUARY": time.February, "FEB": time.February,
		"MARCH": time.March, "MAR": time.March,
		"APRIL": time.April, "APR": time.April,
		"MAY": time.May,
		"JUNE": time.June, "JUN": time.June,
		"JULY": time.July, "JUL": time.July,
		"AUGUST": time.August, "AUG": time.August,
		"SEPTEMBER": time.September, "SEP": time.September,
		"OCTOBER": time.October, "OCT": time.October,
		"NOVEMBER": time.November, "NOV": time.November,
		"DECEMBER": time.December, "DEC": time.December,
	}

	re := regexp.MustCompile(`^([A-Z]+)\s+(\d+)`)
	matches := re.FindStringSubmatch(upper)
	if len(matches) == 3 {
		monthStr := matches[1]
		dayStr := matches[2]
		
		if month, ok := months[monthStr]; ok {
			dayNum, err := strconv.Atoi(dayStr)
			if err == nil {
				t := time.Date(year, month, dayNum, 0, 0, 0, 0, time.UTC)
				return t.YearDay(), nil
			}
		}
	}
	return 0, fmt.Errorf("could not parse date")
}
