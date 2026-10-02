package services

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"fresh-words-backend/models"
)

// ParseHymnDocument parses uploaded DOCX, TXT, or JSON file into Hymn models
func ParseHymnDocument(filePath string, fileBytes []byte) ([]models.Hymn, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".json":
		return parseJSONHymns(fileBytes)
	case ".txt":
		return parseTextHymns(string(fileBytes))
	case ".docx":
		text, err := extractTextFromDocx(fileBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to extract text from DOCX: %w", err)
		}
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("the uploaded DOCX contains mostly images without embedded text. Please upload a formatted DOCX, TXT, or JSON file")
		}
		return parseTextHymns(text)
	default:
		// Attempt JSON first, then text
		if hymns, err := parseJSONHymns(fileBytes); err == nil && len(hymns) > 0 {
			return hymns, nil
		}
		return parseTextHymns(string(fileBytes))
	}
}

func parseJSONHymns(data []byte) ([]models.Hymn, error) {
	type rawHymn struct {
		Number   int         `json:"number"`
		Title    string      `json:"title"`
		Chorus   string      `json:"chorus"`
		Verses   interface{} `json:"verses"`
		Category string      `json:"category"`
		Author   string      `json:"author"`
		Key      string      `json:"key"`
	}

	var rawList []rawHymn
	if err := json.Unmarshal(data, &rawList); err != nil {
		return nil, err
	}

	var result []models.Hymn
	for _, r := range rawList {
		if r.Number <= 0 || strings.TrimSpace(r.Title) == "" {
			continue
		}

		var versesJSON string
		switch v := r.Verses.(type) {
		case string:
			if strings.HasPrefix(strings.TrimSpace(v), "[") {
				versesJSON = strings.TrimSpace(v)
			} else {
				stanzas := strings.Split(v, "\n\n")
				var clean []string
				for _, s := range stanzas {
					if strings.TrimSpace(s) != "" {
						clean = append(clean, strings.TrimSpace(s))
					}
				}
				b, _ := json.Marshal(clean)
				versesJSON = string(b)
			}
		case []interface{}:
			var clean []string
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					clean = append(clean, strings.TrimSpace(s))
				}
			}
			b, _ := json.Marshal(clean)
			versesJSON = string(b)
		case []string:
			b, _ := json.Marshal(v)
			versesJSON = string(b)
		default:
			versesJSON = "[]"
		}

		result = append(result, models.Hymn{
			Number:   r.Number,
			Title:    strings.TrimSpace(r.Title),
			Chorus:   strings.TrimSpace(r.Chorus),
			Verses:   versesJSON,
			Category: strings.TrimSpace(r.Category),
			Author:   strings.TrimSpace(r.Author),
			Key:      strings.TrimSpace(r.Key),
		})
	}

	return result, nil
}

func cleanKey(raw string) string {
	re := regexp.MustCompile(`(?i)^(?:key\s*:\s*)+`)
	return strings.TrimSpace(re.ReplaceAllString(raw, ""))
}

// extractTextFromDocx extracts all text elements from word/document.xml
func extractTextFromDocx(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}

	imageCount := 0
	var docXML []byte
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "word/media/") {
			imageCount++
		}
		if file.Name == "word/document.xml" {
			rc, err := file.Open()
			if err != nil {
				return "", err
			}
			docXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return "", err
			}
		}
	}

	if docXML == nil {
		return "", fmt.Errorf("word/document.xml not found")
	}

	decoder := xml.NewDecoder(bytes.NewReader(docXML))
	var sb strings.Builder
	inText := false

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			if se.Name.Local == "t" {
				inText = true
			} else if se.Name.Local == "p" {
				sb.WriteString("\n")
			}
		case xml.EndElement:
			if se.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				sb.WriteString(string(se))
			}
		}
	}

	extracted := sb.String()
	trimmed := strings.TrimSpace(extracted)
	if len(trimmed) < 100 && imageCount > 0 {
		return "", fmt.Errorf("the document contains %d scanned images without readable text lyrics. Fresh Words Hymnal requires typed text. Please upload a Word document containing selectable text, or a .txt / .json file", imageCount)
	}

	return extracted, nil
}

// parseTextHymns parses plaintext formatted hymn collections
func parseTextHymns(content string) ([]models.Hymn, error) {
	lines := strings.Split(content, "\n")
	var cleanedLines []string
	for _, line := range lines {
		cleanedLines = append(cleanedLines, strings.TrimRight(line, "\r"))
	}

	// Regex to identify hymn headers:
	// e.g. "HYMN 1", "Hymn #1", "1. Amazing Grace", "Hymn 12: Blessed Assurance"
	hymnHeaderRegex := regexp.MustCompile(`(?i)^(?:hymn\s*#?\s*(\d+)[:\.\s]*(.*)|(\d+)\s*[\.:\-]\s*(.+)|(\d+)\s*$)`)

	type rawBlock struct {
		number   int
		title    string
		category string
		author   string
		key      string
		lines    []string
	}

	var blocks []rawBlock
	var current *rawBlock
	currentCategory := "General Hymns"

	for _, line := range cleanedLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if current != nil {
				current.lines = append(current.lines, "")
			}
			continue
		}

		// Check for Category Header (e.g. "ADORATION AND PRAISE", "FAITH AND TRUST")
		if isCategoryHeader(trimmed) {
			currentCategory = formatCategoryTitle(trimmed)
			continue
		}

		matches := hymnHeaderRegex.FindStringSubmatch(trimmed)
		if len(matches) > 0 {
			var numStr string
			var titleStr string

			if matches[1] != "" {
				numStr = matches[1]
				titleStr = strings.TrimSpace(matches[2])
			} else if matches[3] != "" {
				numStr = matches[3]
				titleStr = strings.TrimSpace(matches[4])
			} else if matches[5] != "" {
				numStr = matches[5]
			}

			if num, err := strconv.Atoi(numStr); err == nil && num > 0 {
				if current != nil {
					blocks = append(blocks, *current)
				}
				current = &rawBlock{
					number:   num,
					title:    titleStr,
					category: currentCategory,
					lines:    []string{},
				}
				continue
			}
		}

		if current != nil {
			// Check for metadata tags (Author, Tune/Key)
			lower := strings.ToLower(trimmed)
			if strings.HasPrefix(lower, "author:") {
				current.author = strings.TrimSpace(trimmed[7:])
			} else if strings.HasPrefix(lower, "tune:") || strings.HasPrefix(lower, "key:") || strings.HasPrefix(lower, "meter:") {
				current.key = cleanKey(trimmed)
			} else if strings.HasPrefix(lower, "category:") {
				current.category = strings.TrimSpace(trimmed[9:])
			} else if current.title == "" && !isVerseStart(trimmed) {
				current.title = trimmed
			} else {
				current.lines = append(current.lines, line)
			}
		}
	}

	if current != nil {
		blocks = append(blocks, *current)
	}

	if len(blocks) == 0 {
		return nil, fmt.Errorf("no hymns detected. Ensure each hymn starts with 'Hymn <Number>' or '<Number>. <Title>'")
	}

	var hymns []models.Hymn
	for _, b := range blocks {
		title := b.title
		if title == "" {
			title = fmt.Sprintf("Hymn %d", b.number)
		}

		var verses []string
		var chorusLines []string
		inChorus := false
		var currentVerseLines []string

		verseRegex := regexp.MustCompile(`^(\d+)[\.\s]+(.*)`)
		chorusRegex := regexp.MustCompile(`(?i)^(chorus|refrain)[:\s]*(.*)`)

		for _, l := range b.lines {
			t := strings.TrimSpace(l)
			if t == "" {
				continue
			}

			if cMatch := chorusRegex.FindStringSubmatch(t); len(cMatch) > 0 {
				inChorus = true
				if len(currentVerseLines) > 0 {
					verses = append(verses, strings.Join(currentVerseLines, "\n"))
					currentVerseLines = []string{}
				}
				if cMatch[2] != "" {
					chorusLines = append(chorusLines, cMatch[2])
				}
				continue
			}

			if vMatch := verseRegex.FindStringSubmatch(t); len(vMatch) > 0 {
				inChorus = false
				if len(currentVerseLines) > 0 {
					verses = append(verses, strings.Join(currentVerseLines, "\n"))
					currentVerseLines = []string{}
				}
				currentVerseLines = append(currentVerseLines, t)
				continue
			}

			if inChorus {
				chorusLines = append(chorusLines, t)
			} else {
				currentVerseLines = append(currentVerseLines, t)
			}
		}

		if len(currentVerseLines) > 0 {
			verses = append(verses, strings.Join(currentVerseLines, "\n"))
		}

		// Fallback if no explicit numbered verses found: split by double newlines or group lines
		if len(verses) == 0 {
			joined := strings.TrimSpace(strings.Join(b.lines, "\n"))
			stanzas := strings.Split(joined, "\n\n")
			for _, s := range stanzas {
				if strings.TrimSpace(s) != "" {
					verses = append(verses, strings.TrimSpace(s))
				}
			}
		}

		if verses == nil {
			verses = []string{}
		}
		versesJSON, _ := json.Marshal(verses)
		chorusStr := strings.TrimSpace(strings.Join(chorusLines, "\n"))

		hymns = append(hymns, models.Hymn{
			Number:   b.number,
			Title:    title,
			Chorus:   chorusStr,
			Verses:   string(versesJSON),
			Category: b.category,
			Author:   b.author,
			Key:      cleanKey(b.key),
		})
	}

	return hymns, nil
}

func isVerseStart(line string) bool {
	match, _ := regexp.MatchString(`^(\d+)[\.\s]`, strings.TrimSpace(line))
	return match
}

func isCategoryHeader(line string) bool {
	upper := strings.ToUpper(line)
	known := []string{
		"ADORATION AND PRAISE", "ASSURANCE", "ATONEMENT", "GRACE AND FORGIVENESS",
		"BLOOD OF JESUS", "CHRISTIAN CHARACTER", "CHRISTIAN SERVICE", "CONSECRATION",
		"DEVOTION AND PRAYER", "FAITH AND TRUST", "HEAVEN", "HOLINESS AND PURITY",
		"INVITATION AND REPENTANCE", "JOY, PEACE, LOVE", "PENTECOSTAL, REVIVAL",
		"RAPTURE, RESURRECTION", "THE LORD JESUS CHRIST", "VICTORY AND DELIVERANCE",
	}
	for _, k := range known {
		if strings.Contains(upper, k) {
			return true
		}
	}
	return false
}

func formatCategoryTitle(raw string) string {
	clean := strings.Trim(raw, " .:-1234567890")
	words := strings.Fields(clean)
	var formatted []string
	for _, w := range words {
		lower := strings.ToLower(w)
		if lower == "and" || lower == "of" || lower == "the" {
			formatted = append(formatted, lower)
		} else if len(w) > 0 {
			formatted = append(formatted, strings.ToUpper(w[:1])+strings.ToLower(w[1:]))
		}
	}
	return strings.Join(formatted, " ")
}
