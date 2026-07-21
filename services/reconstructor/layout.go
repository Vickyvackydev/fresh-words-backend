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

	if !doc.IsLinear {
		doc = removeHeadersAndFooters(doc)
		doc = mergeLinesIntoParagraphs(doc)
	}

	doc = mergeDropCaps(doc)

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

		// Compute page right margin for short-line detection
		pageRightMargin := 0.0
		for _, physicalBlock := range page.Blocks {
			for _, line := range physicalBlock.Lines {
				if len(line.Words) > 0 {
					re := line.Words[len(line.Words)-1].X + line.Words[len(line.Words)-1].Width
					if re > pageRightMargin {
						pageRightMargin = re
					}
				}
			}
		}

		for _, physicalBlock := range page.Blocks {
			for _, line := range physicalBlock.Lines {
				lineText := getLineText(line)

				// CRITICAL: If a line starts with a Date or a Heading, we MUST force a block split.
				if isDateStart(lineText) || isHeadingLine(lineText) {
					// BUT wait! If the previous line was ALSO a heading, we are continuing the SAME heading block
					// (e.g. a multi-line title like "GIFTED BUT NOT" / "LIFTED"). We should merge them!
					if lastLine != nil && isHeadingLine(getLineText(*lastLine)) && !isDateStart(lineText) {
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

				lastLineRightEdge := 0.0
				if len(lastLine.Words) > 0 {
					lastLineRightEdge = lastLine.Words[len(lastLine.Words)-1].X + lastLine.Words[len(lastLine.Words)-1].Width
				}

				isShortLine := false
				if pageRightMargin > 0 && (pageRightMargin-lastLineRightEdge) > 70.0 {
					isShortLine = true
				}

				indentX := 0.0
				if len(line.Words) > 0 && len(lastLine.Words) > 0 {
					indentX = line.Words[0].X - lastLine.Words[0].X
				}

				// Split block if there is a large gap, significant indentation, or if the previous line ended early (indicating a paragraph end)
				if gap > avgHeight*1.4 || indentX > avgHeight*1.5 || (isShortLine && gap > avgHeight*1.0) {
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
	re := regexp.MustCompile(`(?i)^(?:DAY\s+\d{1,3}|(?:JANUARY|FEBRUARY|MARCH|APRIL|MAY|JUNE|JULY|AUGUST|SEPTEMBER|OCTOBER|NOVEMBER|DECEMBER|JAN|FEB|MAR|APR|JUN|JUL|AUG|SEP|OCT|NOV|DEC)\s+\d{1,2}(?:ST|ND|RD|TH)?|\d{1,2}(?:ST|ND|RD|TH)?\s+(?:JANUARY|FEBRUARY|MARCH|APRIL|MAY|JUNE|JULY|AUGUST|SEPTEMBER|OCTOBER|NOVEMBER|DECEMBER|JAN|FEB|MAR|APR|JUN|JUL|AUG|SEP|OCT|NOV|DEC))\b`)
	return re.MatchString(upper)
}

func isDateStart(text string) bool {
	upper := strings.ToUpper(strings.TrimSpace(text))
	if upper == "DAY" {
		return true
	}
	if isMonthName(text) {
		return true
	}
	return isDateOrDay(text)
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
	headings := []string{
		"PRAYER", "CONFESSION", "FURTHER READING", "MEMORY VERSE", 
		"KEY POINT", "DECLARATION", "ACTION POINT", "REFLECTION",
		"MESSAGE", "BODY", "SCRIPTURE READING", "BIBLE READING", 
		"BIBLE TEXT", "SCRIPTURE LESSON",
	}
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

// mergeDropCaps detects single-character uppercase blocks/words and merges them cleanly into paragraphs.
func mergeDropCaps(doc *extractor.Document) *extractor.Document {
	for i, page := range doc.Pages {
		var cleanedBlocks []extractor.Block
		for j := 0; j < len(page.Blocks); j++ {
			block := page.Blocks[j]
			blockText := strings.TrimSpace(getBlockText(block))
			
			// Detect drop cap: length 1, uppercase letter as separate block
			if len(blockText) == 1 && blockText >= "A" && blockText <= "Z" && j < len(page.Blocks)-1 {
				nextBlock := page.Blocks[j+1]
				nextText := strings.TrimSpace(getBlockText(nextBlock))
				
				// Check if next block starts with a lowercase letter
				if len(nextText) > 0 && nextText[0] >= 'a' && nextText[0] <= 'z' {
					// Merge separate single-character block into next block
					mergedBlock := mergeTwoBlocks(block, nextBlock, blockText)
					page.Blocks[j+1] = fixMisplacedDropCapsInBlock(mergedBlock)
					continue // skip the current single-character block
				}
			}

			// Clean any inline misplaced drop-caps in this block
			block = fixMisplacedDropCapsInBlock(block)
			cleanedBlocks = append(cleanedBlocks, block)
		}
		doc.Pages[i].Blocks = cleanedBlocks
	}
	return doc
}

func getBlockText(block extractor.Block) string {
	var lines []string
	for _, line := range block.Lines {
		lines = append(lines, getLineText(line))
	}
	return strings.Join(lines, " ")
}

func mergeTwoBlocks(dropCapBlock, mainBlock extractor.Block, dropCapText string) extractor.Block {
	if len(mainBlock.Lines) == 0 {
		return dropCapBlock
	}
	
	firstLine := mainBlock.Lines[0]
	if len(firstLine.Words) == 0 {
		return mainBlock
	}
	
	firstWordText := firstLine.Words[0].Text
	needSpace := false
	if dropCapText == "I" {
		needSpace = true
	} else if dropCapText == "A" {
		directMerges := map[string]bool{
			"nd": true, "s": true, "t": true, "fter": true, "ll": true, 
			"lthough": true, "bout": true, "gainst": true, "n": true, "re": true,
		}
		if !directMerges[firstWordText] {
			needSpace = true
		}
	}
	
	if needSpace {
		newWords := append([]extractor.Word{dropCapBlock.Lines[0].Words[0]}, firstLine.Words...)
		mainBlock.Lines[0].Words = newWords
	} else {
		firstLine.Words[0].Text = dropCapText + firstWordText
		firstLine.Words[0].IsBold = false // clean standard style
	}
	
	mainBlock.Lines[0] = firstLine
	return mainBlock
}

// fixMisplacedDropCapsInBlock fixes inline drop-caps that land at line start or floating inside early sentence text.
func fixMisplacedDropCapsInBlock(block extractor.Block) extractor.Block {
	if len(block.Lines) == 0 || len(block.Lines[0].Words) == 0 {
		return block
	}

	// 1. Check if first line starts with a single uppercase letter word followed by a lowercase word (e.g. ["T", "he"])
	if len(block.Lines[0].Words) >= 2 {
		w0 := block.Lines[0].Words[0].Text
		w1 := block.Lines[0].Words[1].Text

		if len(w0) == 1 && w0 >= "A" && w0 <= "Z" && len(w1) > 0 && w1[0] >= 'a' && w1[0] <= 'z' {
			if w0 == "I" || (w0 == "A" && w1 != "nd" && w1 != "fter" && w1 != "bout" && w1 != "gainst") {
				// Keep space (e.g. "I have", "A man")
			} else {
				// Direct merge (e.g. "T" + "he" -> "The")
				block.Lines[0].Words[0].Text = w0 + w1
				block.Lines[0].Words = append(block.Lines[0].Words[:1], block.Lines[0].Words[2:]...)
			}
		}
	}

	// 2. Check if paragraph starts with a lowercase letter and contains an orphaned capital letter in early lines
	firstText := strings.TrimSpace(getBlockText(block))
	if len(firstText) > 0 && firstText[0] >= 'a' && firstText[0] <= 'z' {
		// Find an isolated uppercase letter in the early words of the block
		var dropCapChar string
		var foundLineIdx, foundWordIdx int = -1, -1

		for lIdx, line := range block.Lines {
			if lIdx > 5 { // search up to first 6 lines for the drop cap
				break
			}
			for wIdx, w := range line.Words {
				if lIdx == 0 && wIdx == 0 {
					continue
				}
				// Isolated single uppercase letter (e.g. "T")
				if len(w.Text) == 1 && w.Text >= "A" && w.Text <= "Z" {
					// If we find one that is significantly larger, we are sure it's the drop cap
					if len(block.Lines) > 0 && len(block.Lines[0].Words) > 0 {
						if w.FontSize > block.Lines[0].Words[0].FontSize*1.3 {
							dropCapChar = w.Text
							foundLineIdx = lIdx
							foundWordIdx = wIdx
							break
						}
					}
					// Otherwise, save it as a fallback if we haven't found any yet
					if dropCapChar == "" {
						dropCapChar = w.Text
						foundLineIdx = lIdx
						foundWordIdx = wIdx
					}
				}
			}
			if dropCapChar != "" && len(block.Lines) > 0 && len(block.Lines[0].Words) > 0 && block.Lines[foundLineIdx].Words[foundWordIdx].FontSize > block.Lines[0].Words[0].FontSize*1.3 {
				break // Found a large one, stop searching
			}
		}

		if dropCapChar != "" && foundLineIdx >= 0 && foundWordIdx >= 0 {
			// Remove the orphaned drop-cap word from its current line
			line := block.Lines[foundLineIdx]
			if foundWordIdx < len(line.Words) {
				line.Words = append(line.Words[:foundWordIdx], line.Words[foundWordIdx+1:]...)
				block.Lines[foundLineIdx] = line
			}

			// Merge the drop-cap character into the very first word of the block
			if len(block.Lines) > 0 && len(block.Lines[0].Words) > 0 {
				firstW := block.Lines[0].Words[0].Text
				if dropCapChar == "I" || (dropCapChar == "A" && firstW != "nd" && firstW != "fter") {
					block.Lines[0].Words = append([]extractor.Word{{Text: dropCapChar}}, block.Lines[0].Words...)
				} else {
					block.Lines[0].Words[0].Text = dropCapChar + firstW
				}
			}
		}
	}

	return block
}

