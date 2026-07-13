package reconstructor

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"fresh-words-backend/services/extractor"
)

// Reconstruct cleans up an extracted document by merging lines into logical paragraphs
// and removing extraneous elements like headers and footers.
func Reconstruct(doc *extractor.Document) (*extractor.Document, error) {
	if doc == nil {
		return nil, fmt.Errorf("cannot reconstruct a nil document")
	}

	if doc.IsLinear {
		// Linear documents (like DOCX) already have native paragraph structure.
		// Bypassing visual layout merging prevents merging paragraphs together.
		return doc, nil
	}

	doc = removeHeadersAndFooters(doc)
	doc = mergeLinesIntoParagraphs(doc)

	return doc, nil
}

// removeHeadersAndFooters detects repeated content at the top and bottom of pages.
func removeHeadersAndFooters(doc *extractor.Document) *extractor.Document {
	for i, page := range doc.Pages {
		var cleanedBlocks []extractor.Block
		
		for _, block := range page.Blocks {
			var cleanedLines []extractor.Line
			for _, line := range block.Lines {
				lineText := getLineText(line)
				lineTextTrimmed := strings.TrimSpace(lineText)

				if lineTextTrimmed == "" {
					continue
				}

				if isPageNumber(lineTextTrimmed) {
					// CRITICAL: Only discard the number if it is at the top/bottom margins of the page.
					if len(line.Words) > 0 {
						y := line.Words[0].Y
						if y >= 50 && y <= 740 {
							cleanedLines = append(cleanedLines, line)
							continue
						}
					}
					continue
				}

				cleanedLines = append(cleanedLines, line)
			}
			if len(cleanedLines) > 0 {
				block.Lines = cleanedLines
				cleanedBlocks = append(cleanedBlocks, block)
			}
		}
		doc.Pages[i].Blocks = cleanedBlocks
	}
	return doc
}

// mergeLinesIntoParagraphs merges physical lines into logical Blocks (Paragraphs).
func mergeLinesIntoParagraphs(doc *extractor.Document) *extractor.Document {
	for i, page := range doc.Pages {
		var logicalBlocks []extractor.Block
		var currentBlock *extractor.Block
		var lastLine *extractor.Line

		for _, physicalBlock := range page.Blocks {
			for _, line := range physicalBlock.Lines {
				lineText := getLineText(line)

				// CRITICAL: If a line starts with a Date or a Heading, we MUST force a block split.
				if isDateOrDay(lineText) || isHeadingLine(lineText) {
					// BUT wait! If the previous line was ALSO a heading, we are continuing the SAME heading block
					// (e.g. a multi-line title like "GIFTED BUT NOT" / "LIFTED"). We should merge them!
					if lastLine != nil && isHeadingLine(getLineText(*lastLine)) && !isDateOrDay(lineText) {
						currentBlock.Lines = append(currentBlock.Lines, line)
						lastLine = &line
						continue
					}

					if currentBlock != nil && len(currentBlock.Lines) > 0 {
						logicalBlocks = append(logicalBlocks, *currentBlock)
					}
					currentBlock = &extractor.Block{}
					currentBlock.Lines = append(currentBlock.Lines, line)
					lastLine = &line
					continue
				}

				// CRITICAL: If the previous line was a Date or a Heading, we MUST also force a split.
				if lastLine != nil {
					lastLineText := getLineText(*lastLine)
					if isDateOrDay(lastLineText) || isHeadingLine(lastLineText) {
						if currentBlock != nil && len(currentBlock.Lines) > 0 {
							logicalBlocks = append(logicalBlocks, *currentBlock)
						}
						currentBlock = &extractor.Block{}
						currentBlock.Lines = append(currentBlock.Lines, line)
						lastLine = &line
						continue
					}
				}

				if currentBlock == nil {
					currentBlock = &extractor.Block{}
					currentBlock.Lines = append(currentBlock.Lines, line)
					lastLine = &line
					continue
				}

				// Calculate vertical gap
				gap := math.Abs(line.Words[0].Y - lastLine.Words[0].Y)
				avgHeight := (line.Words[0].FontSize + lastLine.Words[0].FontSize) / 2.0

				if gap > avgHeight*1.5 {
					if currentBlock != nil && len(currentBlock.Lines) > 0 {
						logicalBlocks = append(logicalBlocks, *currentBlock)
					}
					currentBlock = &extractor.Block{}
					currentBlock.Lines = append(currentBlock.Lines, line)
				} else {
					currentBlock.Lines = append(currentBlock.Lines, line)
				}
				lastLine = &line
			}
		}

		if currentBlock != nil && len(currentBlock.Lines) > 0 {
			logicalBlocks = append(logicalBlocks, *currentBlock)
		}

		doc.Pages[i].Blocks = logicalBlocks
	}
	return doc
}

// Helpers

func getLineText(line extractor.Line) string {
	var words []string
	for _, w := range line.Words {
		words = append(words, w.Text)
	}
	return strings.Join(words, " ")
}

func isPageNumber(s string) bool {
	s = strings.TrimSpace(strings.ReplaceAll(s, "-", ""))
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "page", "")
	s = strings.TrimSpace(s)

	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func isDateOrDay(text string) bool {
	upper := strings.ToUpper(strings.TrimSpace(text))
	re := regexp.MustCompile(`^(JANUARY|FEBRUARY|MARCH|APRIL|MAY|JUNE|JULY|AUGUST|SEPTEMBER|OCTOBER|NOVEMBER|DECEMBER|JAN|FEB|MAR|APR|JUN|JUL|AUG|SEP|OCT|NOV|DEC|DAY)\s+\d{1,2}`)
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

func isHeadingLine(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	upper := strings.ToUpper(trimmed)

	// Check if it's one of our known headings
	headings := []string{"PRAYER", "CONFESSION", "FURTHER READING", "MEMORY VERSE", "KEY POINT", "DECLARATION", "ACTION POINT", "REFLECTION"}
	for _, h := range headings {
		if strings.HasPrefix(upper, h) {
			return true
		}
	}

	// Short fully-uppercase lines are headings (e.g. titles), unless they are just month names or page numbers
	if len(trimmed) < 60 && upper == trimmed && !isPageNumber(trimmed) && !isMonthName(trimmed) {
		hasLetters := false
		for _, c := range trimmed {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				hasLetters = true
				break
			}
		}
		return hasLetters
	}

	return false
}
