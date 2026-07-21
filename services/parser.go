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

	var chunks [][]typography.SemanticBlock
	var currentChunk []typography.SemanticBlock

	// 1. Chunking: split the document by "DAY X" boundaries
	for i := 0; i < len(doc.Blocks); i++ {
		block := doc.Blocks[i]
		text := strings.TrimSpace(block.Text)

		if text == "" {
			continue
		}

		isDate := isDateOrDay(text)
		var mergedText string
		skipNext := false

		// Check if it's a split date (e.g. "June" in one block, "4" in the next)
		if !isDate && (isMonthName(text) || strings.ToUpper(text) == "DAY") {
			if i+1 < len(doc.Blocks) {
				nextText := strings.TrimSpace(doc.Blocks[i+1].Text)
				if isStandaloneDayNumber(nextText) {
					isDate = true
					mergedText = text + " " + nextText
					skipNext = true
				}
			}
		}

		if isDate && (len(text) < 150 || len(mergedText) > 0) {
			if len(currentChunk) > 0 {
				chunks = append(chunks, currentChunk)
			}
			
			if skipNext {
				combinedBlock := block
				combinedBlock.Text = mergedText
				currentChunk = []typography.SemanticBlock{combinedBlock}
				i++ // skip the next block
			} else {
				currentChunk = []typography.SemanticBlock{block}
			}
		} else {
			if len(currentChunk) > 0 {
				currentChunk = append(currentChunk, block)
			}
		}
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	dayCounter := 1
	for _, chunk := range chunks {
		if len(chunk) == 0 {
			continue
		}

		dateText := strings.TrimSpace(chunk[0].Text)
		targetDay := dayCounter
		parsedDay, err := parseDayFromDateString(dateText, year)
		if err == nil && parsedDay > 0 {
			targetDay = parsedDay
			dayCounter = parsedDay + 1
		} else {
			dayCounter++
		}

		log.Printf("[PARSER] --> NEW DEVOTIONAL BOUNDARY FOUND: %s\n", dateText)
		devo := parseDevotionalChunk(chunk, category, targetDay, packageID)
		if devo != nil {
			report.Devotionals = append(report.Devotionals, *devo)
		}
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

// isHeader checks if the text begins with any of the provided header keywords
func isHeader(upper string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.HasPrefix(upper, kw) {
			return true
		}
	}
	return false
}

func parseDevotionalChunk(blocks []typography.SemanticBlock, category string, targetDay int, packageID uuid.UUID) *models.Devotional {
	devo := &models.Devotional{
		ID:         uuid.New(),
		PackageID:  packageID,
		Category:   category,
		DefaultDay: targetDay,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if len(blocks) <= 1 {
		return devo
	}

	// 1. Title is the first block after the Date
	titleIndex := -1
	for i := 1; i < len(blocks); i++ {
		text := strings.TrimSpace(blocks[i].Text)
		if text != "" {
			devo.Title = text
			titleIndex = i
			
			// Look ahead for "PART X"
			if i+1 < len(blocks) {
				nextText := strings.TrimSpace(blocks[i+1].Text)
				if strings.HasPrefix(strings.ToUpper(nextText), "PART ") {
					devo.Title += " " + nextText
					titleIndex = i + 1
				}
			}
			break
		}
	}

	if titleIndex == -1 || titleIndex == len(blocks)-1 {
		return devo
	}

	// 2. Identify explicit sections
	sections := make(map[string][]string)
	currentSection := "Scripture"

	for i := titleIndex + 1; i < len(blocks); i++ {
		text := strings.TrimSpace(blocks[i].Text)
		if text == "" {
			continue
		}
		upper := strings.ToUpper(text)

		if isHeader(upper, "SCRIPTURE READING", "BIBLE READING", "BIBLE TEXT", "SCRIPTURE LESSON") {
			currentSection = "Scripture"
			continue
		}
		if isHeader(upper, "MESSAGE", "BODY") {
			currentSection = "Body"
			continue
		}
		if isHeader(upper, "PRAYER", "PRAYER POINT") {
			currentSection = "Prayer"
			continue
		}
		if isHeader(upper, "REFLECTION", "CONFESSION") {
			currentSection = "Reflection"
			continue
		}
		if isHeader(upper, "ACTION POINT", "KEY POINT") {
			currentSection = "ActionPoint"
			continue
		}
		if isHeader(upper, "MEMORY VERSE") {
			currentSection = "MemoryVerse"
			continue
		}
		if isHeader(upper, "FURTHER READING") {
			currentSection = "FurtherReading"
			continue
		}
		if isHeader(upper, "DECLARATION") {
			currentSection = "Declaration"
			continue
		}

		sections[currentSection] = append(sections[currentSection], text)
	}

	scriptureBlocks := sections["Scripture"]
	bodyBlocks := sections["Body"]

	// 3. Structural Healing: If no explicit MESSAGE header was found, the body accidentally merged into Scripture.
	if len(bodyBlocks) == 0 && len(scriptureBlocks) > 0 {
		splitIdx := -1
		for i := 0; i < len(scriptureBlocks) && i < 3; i++ {
			ref, _, found := extractScripture(scriptureBlocks[i])
			if found && ref != "" {
				// Assuming quote immediately follows reference block if the reference block is very short
				if len(scriptureBlocks[i]) < 50 && i+1 < len(scriptureBlocks) {
					splitIdx = i + 1
				} else {
					splitIdx = i
				}
				break
			}
		}

		if splitIdx != -1 {
			bodyBlocks = scriptureBlocks[splitIdx+1:]
			scriptureBlocks = scriptureBlocks[:splitIdx+1]
		} else {
			// No scripture format matched. Assume first block is quote, rest is body.
			bodyBlocks = scriptureBlocks[1:]
			scriptureBlocks = scriptureBlocks[:1]
		}
	}

	// 4. Extract ref and quote from the identified scripture blocks
	if len(scriptureBlocks) > 0 {
		scriptureText := strings.Join(scriptureBlocks, "\n\n")
		ref, quote, found := extractScripture(scriptureText)
		if found && ref != "" {
			devo.ScriptureReference = ref
			devo.ScriptureQuote = quote
		} else {
			devo.ScriptureQuote = scriptureText
		}
	}

	// 5. Structural Healing 2: If author pasted scripture into the MESSAGE block
	if devo.ScriptureReference == "" && len(bodyBlocks) > 0 {
		ref, quote, found := extractScripture(bodyBlocks[0])
		if found && ref != "" {
			devo.ScriptureReference = ref
			if quote != "" && quote != `""` {
				devo.ScriptureQuote = quote
			}
			bodyBlocks = bodyBlocks[1:] // pop the quote from the body
		}
	}

	// 6. Join sections
	devo.Body = strings.Join(bodyBlocks, "\n\n")
	devo.Prayer = strings.Join(sections["Prayer"], "\n\n")
	devo.Reflection = strings.Join(sections["Reflection"], "\n\n")
	
	actionPoints := strings.Join(sections["ActionPoint"], "\n\n")
	if actionPoints == "" {
		actionPoints = strings.Join(sections["Declaration"], "\n\n")
	}
	devo.ActionPoints = actionPoints

	// Cleanup
	devo.ScriptureQuote = strings.Trim(devo.ScriptureQuote, `"'`)
	if devo.ScriptureQuote == `""` || devo.ScriptureQuote == `"` {
		devo.ScriptureQuote = ""
	}

	return devo
}

// Helpers
func isDateOrDay(text string) bool {
	upper := strings.ToUpper(strings.TrimSpace(text))
	re := regexp.MustCompile(`(?i)^(?:DAY\s+\d{1,3}|(?:JANUARY|FEBRUARY|MARCH|APRIL|MAY|JUNE|JULY|AUGUST|SEPTEMBER|OCTOBER|NOVEMBER|DECEMBER|JAN|FEB|MAR|APR|JUN|JUL|AUG|SEP|OCT|NOV|DEC)\s+\d{1,2}(?:ST|ND|RD|TH)?|\d{1,2}(?:ST|ND|RD|TH)?\s+(?:JANUARY|FEBRUARY|MARCH|APRIL|MAY|JUNE|JULY|AUGUST|SEPTEMBER|OCTOBER|NOVEMBER|DECEMBER|JAN|FEB|MAR|APR|JUN|JUL|AUG|SEP|OCT|NOV|DEC))\b`)
	return re.MatchString(upper)
}

func isMonthName(text string) bool {
	upper := strings.ToUpper(strings.TrimSpace(text))
	months := []string{"JANUARY", "FEBRUARY", "MARCH", "APRIL", "MAY", "JUNE", "JULY", "AUGUST", "SEPTEMBER", "OCTOBER", "NOVEMBER", "DECEMBER", "JAN", "FEB", "MAR", "APR", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}
	for _, m := range months {
		if upper == m {
			return true
		}
	}
	return false
}

func isStandaloneDayNumber(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) == 0 || len(text) > 4 {
		return false
	}
	if text[0] < '0' || text[0] > '9' {
		return false
	}
	for i := 1; i < len(text); i++ {
		c := text[i]
		isDigit := c >= '0' && c <= '9'
		isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !isDigit && !isLetter {
			return false
		}
	}
	return true
}

// extractScripture parses a text block to separate a scripture reference from its quote text.
func extractScripture(text string) (ref string, quote string, found bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", "", false
	}

	// Remove common header prefixes
	headerPrefixes := []string{
		"SCRIPTURE READING:", "SCRIPTURE LESSON:", "BIBLE READING:", "BIBLE TEXT:",
		"SCRIPTURE:", "KEY VERSE:", "MEMORY VERSE:", "SCRIPTURE READING", "BIBLE READING",
	}
	cleanText := trimmed
	for _, p := range headerPrefixes {
		if strings.HasPrefix(strings.ToUpper(cleanText), p) {
			cleanText = strings.TrimSpace(cleanText[len(p):])
			break
		}
	}

	// Match book, chapter and verse (e.g. 1 John 5:18, Luke 18:1, 1 Corinthians 13:4-8, Jeremiah. 33:6, 1John 5:1, Psalms 119: 105)
	re := regexp.MustCompile(`(?i)\(?\b((?:[1-3]\s*)?[A-Z][A-Za-z.]*(?:\s+[A-Za-z.]+)*\s+\d+\s*:\s*\d+(?:\s*[-–—]\s*\d+)?)\)?`)
	loc := re.FindStringSubmatchIndex(cleanText)
	if loc == nil || len(loc) < 4 || loc[0] < 0 || loc[1] > len(cleanText) || loc[2] < 0 || loc[3] > len(cleanText) || loc[2] > loc[3] {
		// Fallback to match book and chapter only (e.g. "1 Chronicles 29-30")
		reNoVerse := regexp.MustCompile(`(?i)\(?\b((?:[1-3]\s*)?[A-Z][A-Za-z.]*(?:\s+[A-Za-z.]+)*\s+\d+(?:\s*[-–—]\s*\d+)?)\)?`)
		loc = reNoVerse.FindStringSubmatchIndex(cleanText)
		if loc == nil || len(loc) < 4 || loc[0] < 0 || loc[1] > len(cleanText) || loc[2] < 0 || loc[3] > len(cleanText) || loc[2] > loc[3] {
			// Check if cleanText is just a quote without an embedded reference
			if strings.HasPrefix(cleanText, `"`) || strings.HasPrefix(cleanText, `“`) || len(cleanText) > 20 {
				return "", strings.Trim(cleanText, `()[]{}""'';,. `), false
			}
			return "", "", false
		}
	}

	ref = cleanText[loc[2]:loc[3]]
	ref = strings.Trim(ref, "()")

	// Remove matched reference to get the quote
	rawQuote := cleanText[:loc[0]] + cleanText[loc[1]:]
	rawQuote = strings.TrimSpace(rawQuote)
	rawQuote = strings.Trim(rawQuote, `()[]{}""'';,. `)
	rawQuote = strings.TrimSpace(rawQuote)

	// Filter out invalid/short non-quote strings (e.g. digits "1", "2" or header remnants)
	if len(rawQuote) <= 3 {
		isOnlyDigits := true
		for _, r := range rawQuote {
			if r < '0' || r > '9' {
				isOnlyDigits = false
				break
			}
		}
		if isOnlyDigits || rawQuote == "" {
			rawQuote = ""
		}
	}

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
