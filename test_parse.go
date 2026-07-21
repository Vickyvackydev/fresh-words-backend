package main

import (
	"fmt"
	"log"

	"fresh-words-backend/services/extractor"
)

func main() {
	pdfPath := `C:\Users\USER\Downloads\DAILY DELIVERANCE.pdf`
	doc, err := extractor.ReadPdf(pdfPath)
	if err != nil {
		log.Fatal(err)
	}

	for p := 0; p < len(doc.Pages); p++ {
		if p > 6 { break }
		for i, b := range doc.Pages[p].Blocks {
			var text string
			for _, l := range b.Lines {
				for _, w := range l.Words {
					text += w.Text + " "
				}
			}
			fmt.Printf("Page %d, Block %d: %q\n", p, i, text)
		}
	}
}
