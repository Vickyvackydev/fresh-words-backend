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

func TestExtractScripture(t *testing.T) {
	tests := []struct {
		input       string
		expectedRef string
		expectedQ   string
	}{
		{
			input:       `SCRIPTURE READING: JOHN 5:18`,
			expectedRef: `JOHN 5:18`,
			expectedQ:   ``,
		},
		{
			input:       `SCRIPTURE READING: 1 JOHN 5:18`,
			expectedRef: `1 JOHN 5:18`,
			expectedQ:   ``,
		},
		{
			input:       `Behold, I will bring it health and cure, and I will cure them, and will reveal unto them the abundance of peace and truth (Jeremiah. 33:6).`,
			expectedRef: `Jeremiah. 33:6`,
			expectedQ:   `Behold, I will bring it health and cure, and I will cure them, and will reveal unto them the abundance of peace and truth`,
		},
		{
			input:       `"We know that whosoever is born of God sinneth not; but he that is begotten of God keepeth himself" (1 John 5:18)`,
			expectedRef: `1 John 5:18`,
			expectedQ:   `We know that whosoever is born of God sinneth not; but he that is begotten of God keepeth himself`,
		},
	}

	for _, tt := range tests {
		ref, q, found := extractScripture(tt.input)
		if !found && tt.expectedRef != "" {
			t.Errorf("extractScripture(%q) found=false; expectedRef=%q", tt.input, tt.expectedRef)
		}
		if ref != tt.expectedRef {
			t.Errorf("extractScripture(%q) ref = %q; want %q", tt.input, ref, tt.expectedRef)
		}
		if q != tt.expectedQ {
			t.Errorf("extractScripture(%q) quote = %q; want %q", tt.input, q, tt.expectedQ)
		}
	}
}
