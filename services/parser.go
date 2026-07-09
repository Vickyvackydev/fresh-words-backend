package services

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fresh-words-backend/models"
	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
)

type ValidationIssue struct {
	DayOfYear int    `json:"day_of_year"`
	DateText  string `json:"date_text"`
	Severity  string `json:"severity"` // "error" or "warning"
	Message   string `json:"message"`
}

type ParserReport struct {
	TotalParsed int               `json:"total_parsed"`
	IsValid     bool              `json:"is_valid"`
	Issues      []ValidationIssue `json:"issues"`
	Devotionals []models.Devotional `json:"-"`
}

// ParseDocument reads the file and extracts a list of devotionals with a validation report.
func ParseDocument(filePath string, category string, year int, packageID uuid.UUID) (*ParserReport, error) {
	var rawText string
	var err error

	if strings.HasSuffix(strings.ToLower(filePath), ".pdf") {
		rawText, err = ReadPdf(filePath)
	} else if strings.HasSuffix(strings.ToLower(filePath), ".docx") {
		rawText, err = ReadDocx(filePath)
	} else {
		return nil, fmt.Errorf("unsupported file format: only PDF and DOCX files are allowed")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read file text: %w", err)
	}

	return parseRawText(rawText, category, year, packageID)
}

// ReadPdf extracts plain text from a PDF file.
func ReadPdf(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	_, err = buf.ReadFrom(b)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ReadDocx extracts text from a DOCX file.
func ReadDocx(path string) (string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			rc, err := file.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return "", err
			}

			xmlStr := string(data)
			// Replace paragraph endings with newlines
			xmlStr = regexp.MustCompile(`</w:p>`).ReplaceAllString(xmlStr, "\n")
			// Strip all XML tags
			re := regexp.MustCompile(`<[^>]*>`)
			plainText := re.ReplaceAllString(xmlStr, "")
			return plainText, nil
		}
	}
	return "", fmt.Errorf("word/document.xml not found in docx zip structure")
}

// parseRawText splits the raw text into daily blocks and parses fields for each devotional.
func parseRawText(rawText string, category string, year int, packageID uuid.UUID) (*ParserReport, error) {
	report := &ParserReport{
		IsValid:     true,
		Issues:      []ValidationIssue{},
		Devotionals: []models.Devotional{},
	}

	// Regex to match date anchors like "January 1" or "December 31"
	reDate := regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2})\b`)
	matches := reDate.FindAllStringSubmatchIndex(rawText, -1)

	if len(matches) == 0 {
		report.IsValid = false
		report.Issues = append(report.Issues, ValidationIssue{
			Severity: "error",
			Message:  "Could not find any daily date headers (e.g. 'January 1') in the document. Please check the layout.",
		})
		return report, nil
	}

	// Track found days to detect duplicates/missing days
	daysFound := make(map[int]string)

	for i := 0; i < len(matches); i++ {
		startIdx := matches[i][0]
		endIdx := len(rawText)
		if i+1 < len(matches) {
			endIdx = matches[i+1][0]
		}

		dayText := rawText[startIdx:endIdx]
		matchSub := reDate.FindStringSubmatch(rawText[matches[i][0]:matches[i][1]])

		if len(matchSub) < 3 {
			continue
		}

		monthName := strings.Title(strings.ToLower(matchSub[1]))
		dayNum, _ := strconv.Atoi(matchSub[2])

		dayOfYear := calculateDayOfYear(monthName, dayNum, year)
		dateStr := fmt.Sprintf("%s %d", monthName, dayNum)

		if dayOfYear <= 0 {
			report.Issues = append(report.Issues, ValidationIssue{
				DateText: dateStr,
				Severity: "warning",
				Message:  fmt.Sprintf("Invalid date header matched: %s", dateStr),
			})
			continue
		}

		if prevDate, dup := daysFound[dayOfYear]; dup {
			report.Issues = append(report.Issues, ValidationIssue{
				DayOfYear: dayOfYear,
				DateText:  dateStr,
				Severity:  "warning",
				Message:   fmt.Sprintf("Duplicate devotional entry detected for day of year %d (%s). Already had %s", dayOfYear, dateStr, prevDate),
			})
		}
		daysFound[dayOfYear] = dateStr

		// Parse the individual block content
		dev, blockIssues := parseDailyBlock(dayText, category, dayOfYear, packageID)
		for _, issue := range blockIssues {
			issue.DayOfYear = dayOfYear
			issue.DateText = dateStr
			report.Issues = append(report.Issues, issue)
			if issue.Severity == "error" {
				report.IsValid = false
			}
		}

		report.Devotionals = append(report.Devotionals, dev)
	}

	report.TotalParsed = len(report.Devotionals)

	// Check if all 365 (or 366) days are present
	expectedDays := 365
	if isLeapYear(year) {
		expectedDays = 366
	}

	if len(daysFound) < expectedDays {
		report.IsValid = false
		report.Issues = append(report.Issues, ValidationIssue{
			Severity: "error",
			Message:  fmt.Sprintf("Missing devotional entries. Expected %d days, but only found %d unique days in the file.", expectedDays, len(daysFound)),
		})
	}

	return report, nil
}

// parseDailyBlock parses a single block of text corresponding to one day using signature-based adjacent line checking.
func parseDailyBlock(blockText string, category string, dayOfYear int, packageID uuid.UUID) (models.Devotional, []ValidationIssue) {
	issues := []ValidationIssue{}
	dev := models.Devotional{
		ID:        uuid.New(),
		PackageID: packageID,
		Category:  category,
		DefaultDay: dayOfYear,
	}

	rawLines := strings.Split(blockText, "\n")
	reRef := regexp.MustCompile(`\((?i)(?:[1-3]\s+)?[A-Z][a-z]+(?:\s+[A-Z][a-z]+)*\s+\d+:\d+(?:-\d+)?\)`)
	rePage := regexp.MustCompile(`^\d+$`)
	reDate := regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+(\d{1,2})\b`)

	// Clean lines to non-empty, non-page, non-date-header lines
	type LineInfo struct {
		text  string
		index int
	}
	var lines []LineInfo
	for idx, rl := range rawLines {
		line := strings.TrimSpace(rl)
		if line == "" {
			continue
		}
		if rePage.MatchString(line) {
			continue
		}
		if reDate.MatchString(line) {
			continue
		}
		lines = append(lines, LineInfo{text: line, index: idx})
	}

	if len(lines) == 0 {
		issues = append(issues, ValidationIssue{
			Severity: "error",
			Message:  "Empty or extremely short devotional entry.",
		})
		return dev, issues
	}

	// 1. Find Title candidate lines (completely uppercase, length < 150, not containing scripture ref)
	var titleIndices []int
	for i, l := range lines {
		cleanLine := regexp.MustCompile(`[^a-zA-Z]`).ReplaceAllString(l.text, "")
		isUppercase := len(cleanLine) > 0 && cleanLine == strings.ToUpper(cleanLine)
		hasScriptureRef := reRef.MatchString(l.text)

		if isUppercase && len(l.text) < 150 && !hasScriptureRef {
			titleIndices = append(titleIndices, i)
		}
	}

	// Group adjacent title lines (if any)
	var titleStart, titleEnd = -1, -1
	if len(titleIndices) > 0 {
		titleStart = titleIndices[0]
		titleEnd = titleIndices[0]
		for i := 1; i < len(titleIndices); i++ {
			if titleIndices[i] == titleEnd+1 {
				titleEnd = titleIndices[i]
			} else {
				break
			}
		}
	}

	// 2. Identify the Main Scripture line based on adjacency to the Title
	mainScriptureIndex := -1
	if titleStart != -1 {
		beforeIdx := titleStart - 1
		afterIdx := titleEnd + 1

		hasBefore := beforeIdx >= 0 && reRef.MatchString(lines[beforeIdx].text)
		hasAfter := afterIdx < len(lines) && reRef.MatchString(lines[afterIdx].text)

		if hasBefore && hasAfter {
			// If both match, pick the shorter one (body paragraph is much longer)
			if len(lines[beforeIdx].text) < len(lines[afterIdx].text) {
				mainScriptureIndex = beforeIdx
			} else {
				mainScriptureIndex = afterIdx
			}
		} else if hasBefore {
			if len(lines[beforeIdx].text) < 400 {
				mainScriptureIndex = beforeIdx
			}
		} else if hasAfter {
			if len(lines[afterIdx].text) < 400 {
				mainScriptureIndex = afterIdx
			}
		}
	}

	// Fallback: Look for the first line in the block that has a scripture reference and is under 400 characters
	if mainScriptureIndex == -1 {
		for i, l := range lines {
			if i != titleStart && i != titleEnd && reRef.MatchString(l.text) && len(l.text) < 400 {
				mainScriptureIndex = i
				break
			}
		}
	}

	// 3. Reconstruct fields
	var titleLines []string
	if titleStart != -1 {
		for idx := titleStart; idx <= titleEnd; idx++ {
			titleLines = append(titleLines, lines[idx].text)
		}
		dev.Title = strings.Join(titleLines, " ")
	} else {
		issues = append(issues, ValidationIssue{
			Severity: "warning",
			Message:  "Could not identify the devotional title.",
		})
		dev.Title = "UNTITLED DEVOTIONAL"
	}

	if mainScriptureIndex != -1 {
		scriptureLine := lines[mainScriptureIndex].text
		refMatch := reRef.FindString(scriptureLine)
		dev.ScriptureReference = strings.Trim(refMatch, "()")

		quote := strings.TrimSpace(strings.Replace(scriptureLine, refMatch, "", -1))
		quote = strings.TrimSpace(reRef.ReplaceAllString(quote, ""))
		dev.ScriptureQuote = quote
	} else {
		issues = append(issues, ValidationIssue{
			Severity: "warning",
			Message:  "Could not find a valid scripture reference (e.g. '(John 3:16)').",
		})
		dev.ScriptureReference = "Scripture Reference"
	}

	// 4. Construct Body and parse sub-sections (Prayer, Reflection, Action Points)
	var bodyLines []string
	var prayerLines []string
	var reflectionLines []string
	var actionPoints []string

	currentSection := "body"

	for i, l := range lines {
		if (titleStart != -1 && i >= titleStart && i <= titleEnd) || i == mainScriptureIndex {
			continue
		}
		line := l.text
		lowerLine := strings.ToLower(line)

		if strings.HasPrefix(lowerLine, "prayer:") {
			currentSection = "prayer"
			line = strings.TrimSpace(l.text[7:])
			if line != "" {
				prayerLines = append(prayerLines, line)
			}
		} else if strings.HasPrefix(lowerLine, "reflection:") {
			currentSection = "reflection"
			line = strings.TrimSpace(l.text[11:])
			if line != "" {
				reflectionLines = append(reflectionLines, line)
			}
		} else if strings.HasPrefix(lowerLine, "action points:") || strings.HasPrefix(lowerLine, "action point:") {
			currentSection = "action"
			line = strings.TrimSpace(strings.TrimPrefix(lowerLine, "action points:"))
			line = strings.TrimSpace(strings.TrimPrefix(line, "action point:"))
			if line != "" {
				actionPoints = append(actionPoints, line)
			}
		} else {
			switch currentSection {
			case "body":
				bodyLines = append(bodyLines, line)
			case "prayer":
				prayerLines = append(prayerLines, line)
			case "reflection":
				reflectionLines = append(reflectionLines, line)
			case "action":
				cleanLine := regexp.MustCompile(`^[-*\d\.\s]+`).ReplaceAllString(line, "")
				if cleanLine != "" {
					actionPoints = append(actionPoints, cleanLine)
				}
			}
		}
	}

	if len(bodyLines) == 0 {
		issues = append(issues, ValidationIssue{
			Severity: "error",
			Message:  "Devotional body text is missing.",
		})
	} else {
		// Clean up drop caps
		firstLine := bodyLines[0]
		if len(firstLine) > 2 && firstLine[1] == ' ' && firstLine[0] >= 'A' && firstLine[0] <= 'Z' && firstLine[2] >= 'a' && firstLine[2] <= 'z' {
			bodyLines[0] = string(firstLine[0]) + firstLine[2:]
		}
		dev.Body = strings.Join(bodyLines, "\n\n")
	}

	if len(prayerLines) > 0 {
		dev.Prayer = strings.Join(prayerLines, " ")
	}
	if len(reflectionLines) > 0 {
		dev.Reflection = strings.Join(reflectionLines, " ")
	}

	// Serialize action points to JSON array
	if len(actionPoints) > 0 {
		var apBuilder strings.Builder
		apBuilder.WriteString("[")
		for i, ap := range actionPoints {
			apBuilder.WriteString(fmt.Sprintf("%q", ap))
			if i+1 < len(actionPoints) {
				apBuilder.WriteString(",")
			}
		}
		apBuilder.WriteString("]")
		dev.ActionPoints = apBuilder.String()
	}

	return dev, issues
}

func calculateDayOfYear(month string, day int, year int) int {
	var m time.Month
	switch strings.ToLower(month) {
	case "january":
		m = time.January
	case "february":
		m = time.February
	case "march":
		m = time.March
	case "april":
		m = time.April
	case "may":
		m = time.May
	case "june":
		m = time.June
	case "july":
		m = time.July
	case "august":
		m = time.August
	case "september":
		m = time.September
	case "october":
		m = time.October
	case "november":
		m = time.November
	case "december":
		m = time.December
	default:
		return -1
	}

	t := time.Date(year, m, day, 0, 0, 0, 0, time.UTC)
	if t.Year() != year || t.Month() != m || t.Day() != day {
		return -1
	}
	return t.YearDay()
}

func splitIntoLines(text string) []string {
	// Normalize line endings
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(normalized, "\n")
}
