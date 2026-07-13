package normalizer

import (
	"regexp"
	"strings"

	"fresh-words-backend/services/typography"
)

// Normalize processes a SemanticDocument and applies normalization rules.
func Normalize(doc *typography.SemanticDocument) *typography.SemanticDocument {
	for i, block := range doc.Blocks {
		doc.Blocks[i].Text = NormalizeBibleReference(block.Text)
	}
	return doc
}

// NormalizeBibleReference standardizes split, abbreviated, or mistyped Bible references.
func NormalizeBibleReference(text string) string {
	// Standardize dot as colon between numbers (e.g. "6.12" -> "6:12")
	text = regexp.MustCompile(`\b(\d+)\.(\d+)\b`).ReplaceAllString(text, "$1:$2")

	// Standardize colons after book names (e.g. "1Tim:6:12" -> "1 Tim 6:12")
	text = regexp.MustCompile(`\b(1|2|3|I|II|III)?\s*([A-Za-z]+)\s*:\s*(\d+)`).ReplaceAllString(text, "$1 $2 $3")

	// Remove excessive whitespaces or weird PDF line break artifact spacing
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")

	// Common book mappings
	bookMap := map[string]string{
		"Gen": "Genesis", "Gen.": "Genesis",
		"Ex": "Exodus", "Exod": "Exodus",
		"Lev": "Leviticus",
		"Num": "Numbers",
		"Deut": "Deuteronomy",
		"Josh": "Joshua",
		"Judg": "Judges",
		"Ruth": "Ruth",
		"1 Sam": "1 Samuel", "I Sam": "1 Samuel",
		"2 Sam": "2 Samuel", "II Sam": "2 Samuel",
		"1 Kgs": "1 Kings", "1 Kings": "1 Kings",
		"2 Kgs": "2 Kings", "2 Kings": "2 Kings",
		"1 Chr": "1 Chronicles", "1 Chron": "1 Chronicles",
		"2 Chr": "2 Chronicles", "2 Chron": "2 Chronicles",
		"Ezra": "Ezra",
		"Neh": "Nehemiah",
		"Esth": "Esther",
		"Job": "Job",
		"Ps": "Psalm", "Psalms": "Psalm", "Psa": "Psalm",
		"Prov": "Proverbs",
		"Eccl": "Ecclesiastes",
		"Song": "Song of Solomon", "Song of Songs": "Song of Solomon",
		"Isa": "Isaiah",
		"Jer": "Jeremiah",
		"Lam": "Lamentations",
		"Ezek": "Ezekiel",
		"Dan": "Daniel",
		"Hos": "Hosea",
		"Joel": "Joel",
		"Amos": "Amos",
		"Obad": "Obadiah",
		"Jonah": "Jonah",
		"Mic": "Micah",
		"Nah": "Nahum",
		"Hab": "Habakkuk",
		"Zeph": "Zephaniah",
		"Hag": "Haggai",
		"Zech": "Zechariah",
		"Mal": "Malachi",
		"Matt": "Matthew",
		"Mark": "Mark",
		"Luke": "Luke",
		"John": "John",
		"Acts": "Acts",
		"Rom": "Romans",
		"1 Cor": "1 Corinthians", "I Cor": "1 Corinthians", "1 Cor.": "1 Corinthians",
		"2 Cor": "2 Corinthians", "II Cor": "2 Corinthians", "2 Cor.": "2 Corinthians",
		"Gal": "Galatians",
		"Eph": "Ephesians",
		"Phil": "Philippians",
		"Col": "Colossians",
		"1 Thess": "1 Thessalonians",
		"2 Thess": "2 Thessalonians",
		"1 Tim": "1 Timothy", "1Tim": "1 Timothy",
		"2 Tim": "2 Timothy", "2Tim": "2 Timothy",
		"Titus": "Titus",
		"Phlm": "Philemon",
		"Heb": "Hebrews",
		"Jas": "James",
		"1 Pet": "1 Peter",
		"2 Pet": "2 Peter",
		"1 John": "1 John",
		"2 John": "2 John",
		"3 John": "3 John",
		"Jude": "Jude",
		"Rev": "Revelation",
	}

	for abbr, full := range bookMap {
		regexPattern := `(?i)\b` + strings.ReplaceAll(abbr, ".", `\.`) + `\b`
		re := regexp.MustCompile(regexPattern)
		text = re.ReplaceAllString(text, full)
	}

	// Fix split numbers like "1 Corinthians 13 : 4" -> "1 Corinthians 13:4"
	text = regexp.MustCompile(`(\d+)\s*:\s*(\d+)`).ReplaceAllString(text, "$1:$2")

	return text
}
