package extractor

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// ReadDocx extracts layout-aware blocks from a DOCX file without using JVM.
func ReadDocx(path string) (*Document, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("could not open DOCX file: %w", err)
	}
	defer reader.Close()

	var docXML []byte
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			rc, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("could not open word/document.xml: %w", err)
			}
			docXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("could not read word/document.xml: %w", err)
			}
			break
		}
	}

	if docXML == nil {
		return nil, fmt.Errorf("word/document.xml not found in DOCX zip structure")
	}

	decoder := xml.NewDecoder(bytes.NewReader(docXML))

	result := &Document{}
	page := Page{Number: 1}
	currentY := 0.0

	type xmlParagraph struct {
		words []Word
	}

	var currentParagraph *xmlParagraph

	// Keep track of active run properties
	var currentFont string
	var currentSize float64 = 11.0
	var isBold bool
	var isItalic bool

	inRun := false
	inText := false
	inRPr := false

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to decode XML sequentially: %w", err)
		}

		switch se := token.(type) {
		case xml.StartElement:
			localName := se.Name.Local
			switch localName {
			case "p":
				currentParagraph = &xmlParagraph{}
			case "r":
				inRun = true
				currentFont = ""
				currentSize = 11.0
				isBold = false
				isItalic = false
			case "rPr":
				inRPr = true
			case "b":
				if inRPr {
					isBold = true
				}
			case "i":
				if inRPr {
					isItalic = true
				}
			case "sz":
				if inRPr {
					for _, attr := range se.Attr {
						if attr.Name.Local == "val" {
							var val float64
							if _, err := fmt.Sscanf(attr.Value, "%f", &val); err == nil {
								currentSize = val / 2.0
							}
						}
					}
				}
			case "rFonts":
				if inRPr {
					for _, attr := range se.Attr {
						if attr.Name.Local == "ascii" {
							currentFont = attr.Value
						}
					}
				}
			case "t":
				if inRun {
					inText = true
				}
			}

		case xml.EndElement:
			localName := se.Name.Local
			switch localName {
			case "p":
				if currentParagraph != nil {
					// We finished a paragraph. If it has words, wrap it into a Block/Line
					if len(currentParagraph.words) > 0 {
						var block Block
						var currentLine Line
						currentX := 0.0
						for _, w := range currentParagraph.words {
							w.X = currentX
							w.Y = currentY
							currentLine.Words = append(currentLine.Words, w)
							currentX += w.Width + (w.FontSize * 0.5)
						}
						block.Lines = append(block.Lines, currentLine)
						page.Blocks = append(page.Blocks, block)
						currentY += 15.0
					}
					currentParagraph = nil
				}
			case "r":
				inRun = false
			case "rPr":
				inRPr = false
			case "t":
				inText = false
			}

		case xml.CharData:
			if inText && currentParagraph != nil && inRun {
				textVal := string(se)
				// Split into words by spaces (retaining the logic from original ReadDocx)
				tokens := strings.Split(textVal, " ")
				for i, tokenStr := range tokens {
					if tokenStr == "" && i != len(tokens)-1 {
						continue
					}
					if tokenStr != "" {
						word := Word{
							Text:     tokenStr,
							Width:    float64(len(tokenStr)) * (currentSize * 0.5),
							Height:   currentSize,
							Page:     1,
							FontName: currentFont,
							FontSize: currentSize,
							IsBold:   isBold,
							IsItalic: isItalic,
						}
						currentParagraph.words = append(currentParagraph.words, word)
					}
				}
			}
		}
	}

	result.IsLinear = true
	result.Pages = append(result.Pages, page)
	return result, nil
}

