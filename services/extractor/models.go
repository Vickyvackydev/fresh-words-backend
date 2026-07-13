package extractor

// Document represents the full hierarchical structure of an extracted document.
type Document struct {
	Pages    []Page
	IsLinear bool
}

// Page represents a single page in the document.
type Page struct {
	Number int
	Blocks []Block
}

// Block represents a distinct bounding box of text (often a paragraph or heading).
type Block struct {
	Lines []Line
}

// Line represents a horizontal sequence of words on the same vertical baseline.
type Line struct {
	Words []Word
}

// Word represents a single word with its physical positioning and typography.
type Word struct {
	Text     string
	X        float64
	Y        float64
	Width    float64
	Height   float64
	Page     int
	FontName string
	FontSize float64
	IsBold   bool
	IsItalic bool
}

// FullText returns the full plain text of the document.
func (d *Document) FullText() string {
	var out string
	for _, page := range d.Pages {
		for _, block := range page.Blocks {
			for _, line := range block.Lines {
				for i, word := range line.Words {
					out += word.Text
					if i < len(line.Words)-1 {
						out += " "
					}
				}
				out += "\n"
			}
			out += "\n"
		}
	}
	return out
}
