package extractor

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ReadPdf reads a PDF file and extracts a layout-aware Document natively in Go.
func ReadPdf(path string) (*Document, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	r, err := pdf.NewReader(file, stat.Size())
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF: %w", err)
	}

	result := &Document{}
	numPages := r.NumPage()

	for i := 1; i <= numPages; i++ {
		page := r.Page(i)
		texts := page.Content().Text

		pageObj := Page{Number: i}
		
		words := groupTextsIntoWords(texts, i)
		
		// CRITICAL: Sort words top-to-bottom (Y descending), then left-to-right (X ascending)
		// ledongthuc/pdf returns characters in random drawing order, so we must sort them to get reading order.
		// Using TopY (Y + FontSize * 0.8) accounts for font ascenders and prevents large drop caps from overshooting to the previous line.
		sort.Slice(words, func(a, b int) bool {
			topA := words[a].Y + words[a].FontSize*0.8
			topB := words[b].Y + words[b].FontSize*0.8
			minFont := math.Min(words[a].FontSize, words[b].FontSize)

			// If they are roughly on the same horizontal line
			if math.Abs(topA-topB) < minFont*0.8 {
				return words[a].X < words[b].X
			}
			
			// Otherwise sort top-to-bottom
			return topA > topB
		})

		lines := groupWordsIntoLines(words)

		block := Block{Lines: lines}
		pageObj.Blocks = append(pageObj.Blocks, block)

		result.Pages = append(result.Pages, pageObj)
	}

	return result, nil
}

// groupTextsIntoWords groups individual character strings into discrete words.
func groupTextsIntoWords(texts []pdf.Text, pageNum int) []Word {
	var words []Word
	var currentWord Word
	var wordText strings.Builder

	for _, c := range texts {
		fontName := c.Font
		fontSize := c.FontSize

		// Explicit Space
		if c.S == " " || c.S == "\n" {
			if wordText.Len() > 0 {
				currentWord.Text = wordText.String()
				words = append(words, currentWord)
				wordText.Reset()
			}
			continue
		}

		// Gap-based space detection (if there is a significant gap between characters on the same line)
		if wordText.Len() > 0 {
			if math.Abs(currentWord.Y-c.Y) < fontSize*0.5 {
				gap := c.X - (currentWord.X + currentWord.Width)
				if gap > (fontSize*0.25) || gap < -0.1 {
					currentWord.Text = wordText.String()
					words = append(words, currentWord)
					wordText.Reset()
				}
			} else {
				currentWord.Text = wordText.String()
				words = append(words, currentWord)
				wordText.Reset()
			}
		}

		if wordText.Len() == 0 {
			currentWord = Word{
				X:        c.X,
				Y:        c.Y,
				Page:     pageNum,
				FontName: fontName,
				FontSize: fontSize,
				IsBold:   strings.Contains(strings.ToLower(fontName), "bold"),
				IsItalic: strings.Contains(strings.ToLower(fontName), "italic"),
			}
		}

		currentWord.Width = c.X + c.W - currentWord.X
		currentWord.Height = fontSize
		wordText.WriteString(c.S)
	}

	if wordText.Len() > 0 {
		currentWord.Text = wordText.String()
		words = append(words, currentWord)
	}

	return words
}

// groupWordsIntoLines groups words that fall on the same horizontal baseline (now using TopY).
func groupWordsIntoLines(words []Word) []Line {
	var lines []Line
	if len(words) == 0 {
		return lines
	}

	var currentLine Line
	var lineTopY float64

	for _, w := range words {
		topY := w.Y + w.FontSize*0.8

		var isSameLine bool
		if len(currentLine.Words) == 0 {
			isSameLine = true
			lineTopY = topY
		} else {
			minFont := math.Min(w.FontSize, currentLine.Words[0].FontSize)
			isSameLine = math.Abs(topY-lineTopY) < minFont*0.8
		}

		if !isSameLine {
			lines = append(lines, currentLine)
			currentLine = Line{}
			lineTopY = topY
		}
		currentLine.Words = append(currentLine.Words, w)
	}

	if len(currentLine.Words) > 0 {
		lines = append(lines, currentLine)
	}

	return lines
}
