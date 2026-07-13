package services

import (
	"testing"

	"fresh-words-backend/services/normalizer"
)

func TestNormalizeBibleReference(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1 Cor 13 : 4", "1 Corinthians 13:4"},
		{"Gen 1:1", "Genesis 1:1"},
		{"1 \n Cor 13:4", "1 Corinthians 13:4"},
		{"Psalm 23", "Psalm 23"},
		{"Ps 91: 1", "Psalm 91:1"},
	}

	for _, tt := range tests {
		result := normalizer.NormalizeBibleReference(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeBibleReference(%q) = %q; want %q", tt.input, result, tt.expected)
		}
	}
}

// TODO: Add tests with mock SemanticDocuments for extractDevotionals
