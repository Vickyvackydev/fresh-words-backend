package typography

import (
	"math"
	"strings"

	"fresh-words-backend/services/extractor"
)

// BlockType defines the semantic type of a layout block
type BlockType string

const (
	TypeParagraph BlockType = "Paragraph"
	TypeHeading   BlockType = "Heading"
)

// SemanticBlock wraps an extractor.Block with semantic meaning.
type SemanticBlock struct {
	extractor.Block
	Type BlockType
	Text string
}

// SemanticDocument is the parsed tree of semantic blocks.
type SemanticDocument struct {
	Blocks []SemanticBlock
}

// Detect analyzes the typography of a reconstructed Document and returns a SemanticDocument.
func Detect(doc *extractor.Document) *SemanticDocument {
	// First pass: find the dominant font size
	dominantFontSize := findDominantFontSize(doc)

	var semanticDoc SemanticDocument

	for _, page := range doc.Pages {
		for _, block := range page.Blocks {
			text := getBlockText(block)
			if strings.TrimSpace(text) == "" {
				continue
			}

			blockType := TypeParagraph
			avgFontSize, isBold, isAllcaps := analyzeBlockTypography(block)

			// If the text is larger than dominant font, or bold, or all caps, it's likely a heading
			if avgFontSize > dominantFontSize+1.0 || isBold || (isAllcaps && len(text) > 3) {
				blockType = TypeHeading
			}

			// Short standalone sentences are also often headings in devotionals
			if len(text) < 50 && isBold {
				blockType = TypeHeading
			}

			semanticDoc.Blocks = append(semanticDoc.Blocks, SemanticBlock{
				Block: block,
				Type:  blockType,
				Text:  text,
			})
		}
	}

	return &semanticDoc
}

func findDominantFontSize(doc *extractor.Document) float64 {
	sizeCounts := make(map[int]int) // rounded to int for stable counting
	
	for _, page := range doc.Pages {
		for _, block := range page.Blocks {
			for _, line := range block.Lines {
				for _, word := range line.Words {
					rounded := int(math.Round(word.FontSize))
					sizeCounts[rounded]++
				}
			}
		}
	}

	maxCount := 0
	dominant := 11 // fallback
	for size, count := range sizeCounts {
		if count > maxCount {
			maxCount = count
			dominant = size
		}
	}

	return float64(dominant)
}

func analyzeBlockTypography(block extractor.Block) (avgFontSize float64, isBold bool, isAllcaps bool) {
	totalWords := 0
	boldWords := 0
	totalSize := 0.0
	isAllcaps = true

	for _, line := range block.Lines {
		for _, word := range line.Words {
			if strings.TrimSpace(word.Text) == "" {
				continue
			}
			
			totalWords++
			totalSize += word.FontSize
			if word.IsBold {
				boldWords++
			}
			if strings.ToUpper(word.Text) != word.Text {
				isAllcaps = false
			}
		}
	}

	if totalWords > 0 {
		avgFontSize = totalSize / float64(totalWords)
		isBold = float64(boldWords)/float64(totalWords) > 0.5 // if more than half is bold
	} else {
		isAllcaps = false
	}

	return
}

func getBlockText(block extractor.Block) string {
	var words []string
	for _, line := range block.Lines {
		for _, w := range line.Words {
			words = append(words, w.Text)
		}
	}
	return strings.Join(words, " ")
}
