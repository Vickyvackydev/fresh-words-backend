package extractor

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// wDocument represents the root of a word/document.xml file
type wDocument struct {
	XMLName xml.Name `xml:"document"`
	Body    wBody    `xml:"body"`
}

type wBody struct {
	Paragraphs []wParagraph `xml:"p"`
}

type wParagraph struct {
	Runs []wRun `xml:"r"`
}

type wRun struct {
	Properties *wRunProperties `xml:"rPr"`
	Texts      []string        `xml:"t"`
}

type wRunProperties struct {
	Bold   *struct{} `xml:"b"`
	Italic *struct{} `xml:"i"`
	Size   *wSize    `xml:"sz"`
	Font   *wFont    `xml:"rFonts"`
}

type wSize struct {
	Val string `xml:"val,attr"`
}

type wFont struct {
	Ascii string `xml:"ascii,attr"`
}

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

	// CRITICAL: Strip "w:" namespace prefix from tags and attributes.
	// Go's xml.Unmarshal is namespace-aware and will ignore <w:p> unless we specify 
	// the full OpenXML namespace on every tag. Stripping namespaces is clean, fast, and works instantly.
	xmlStr := string(docXML)
	xmlStr = strings.ReplaceAll(xmlStr, "<w:", "<")
	xmlStr = strings.ReplaceAll(xmlStr, "</w:", "</")
	xmlStr = strings.ReplaceAll(xmlStr, " w:", " ")
	docXML = []byte(xmlStr)

	var wDoc wDocument
	if err := xml.Unmarshal(docXML, &wDoc); err != nil {
		return nil, fmt.Errorf("failed to parse DOCX XML: %w", err)
	}

	result := &Document{}
	page := Page{Number: 1}
	currentY := 0.0

	for _, p := range wDoc.Body.Paragraphs {
		var block Block
		var currentLine Line
		currentX := 0.0

		for _, r := range p.Runs {
			fontName := ""
			fontSize := 11.0
			isBold := false
			isItalic := false

			if r.Properties != nil {
				if r.Properties.Font != nil {
					fontName = r.Properties.Font.Ascii
				}
				if r.Properties.Size != nil {
					if sz, err := strconv.ParseFloat(r.Properties.Size.Val, 64); err == nil {
						fontSize = sz / 2.0
					}
				}
				isBold = r.Properties.Bold != nil
				isItalic = r.Properties.Italic != nil
			}

			for _, t := range r.Texts {
				tokens := strings.Split(t, " ")
				for i, token := range tokens {
					if token == "" && i != len(tokens)-1 {
						continue
					}

					if token != "" {
						word := Word{
							Text:     token,
							X:        currentX,
							Y:        currentY,
							Width:    float64(len(token)) * (fontSize * 0.5),
							Height:   fontSize,
							Page:     1,
							FontName: fontName,
							FontSize: fontSize,
							IsBold:   isBold,
							IsItalic: isItalic,
						}
						currentLine.Words = append(currentLine.Words, word)
						currentX += word.Width + (fontSize * 0.5)
					}
				}
			}
		}

		if len(currentLine.Words) > 0 {
			block.Lines = append(block.Lines, currentLine)
			page.Blocks = append(page.Blocks, block)
			currentY += 15.0
		}
	}

	result.IsLinear = true
	result.Pages = append(result.Pages, page)
	return result, nil
}
