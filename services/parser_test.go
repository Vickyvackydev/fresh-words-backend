package services

import (
	"testing"

	"github.com/google/uuid"
)

func TestCalculateDayOfYear(t *testing.T) {
	tests := []struct {
		month    string
		day      int
		year     int
		expected int
	}{
		{"January", 1, 2026, 1},
		{"January", 31, 2026, 31},
		{"February", 1, 2026, 32},
		{"February", 28, 2026, 59},
		{"February", 29, 2024, 60}, // Leap year
		{"December", 31, 2026, 365},
		{"December", 31, 2024, 366}, // Leap year
		{"Invalid", 1, 2026, -1},
	}

	for _, tt := range tests {
		result := calculateDayOfYear(tt.month, tt.day, tt.year)
		if result != tt.expected {
			t.Errorf("calculateDayOfYear(%q, %d, %d) = %d; want %d", tt.month, tt.day, tt.year, result, tt.expected)
		}
	}
}

func TestParseDailyBlock(t *testing.T) {
	blockText := `January 1
WHAT PRAYER CANNOT DO, MORE PRAYER WILL DO

"And he spoke a parable unto them to this end, that men ought always to pray, and not to faint." (Luke 18:1)

O ur meditation for today is an all too common experience in the school of prayer. There are times we pray and receive the answer instantaneously.

PRAYER: Lord, help us to pray and fire on.
REFLECTION: Pray until something happens.
ACTION POINTS:
- Pray every morning.
- Trust in God.
`

	category := "prayer"
	dayOfYear := 1
	packageID := uuid.New()

	dev, issues := parseDailyBlock(blockText, category, dayOfYear, packageID)

	if len(issues) > 0 {
		t.Errorf("Expected 0 parsing issues, got %d: %v", len(issues), issues)
	}

	if dev.Title != "WHAT PRAYER CANNOT DO, MORE PRAYER WILL DO" {
		t.Errorf("Parsed Title = %q; want %q", dev.Title, "WHAT PRAYER CANNOT DO, MORE PRAYER WILL DO")
	}

	if dev.ScriptureReference != "Luke 18:1" {
		t.Errorf("Parsed ScriptureReference = %q; want %q", dev.ScriptureReference, "Luke 18:1")
	}

	expectedQuote := `"And he spoke a parable unto them to this end, that men ought always to pray, and not to faint."`
	if dev.ScriptureQuote != expectedQuote {
		t.Errorf("Parsed ScriptureQuote = %q; want %q", dev.ScriptureQuote, expectedQuote)
	}

	expectedBodyPrefix := "Our meditation for today"
	if dev.Body[:24] != expectedBodyPrefix {
		t.Errorf("Parsed Body start = %q; want prefix %q", dev.Body[:24], expectedBodyPrefix)
	}

	if dev.Prayer != "Lord, help us to pray and fire on." {
		t.Errorf("Parsed Prayer = %q; want %q", dev.Prayer, "Lord, help us to pray and fire on.")
	}

	if dev.Reflection != "Pray until something happens." {
		t.Errorf("Parsed Reflection = %q; want %q", dev.Reflection, "Pray until something happens.")
	}

	expectedActionPoints := `["Pray every morning.","Trust in God."]`
	if dev.ActionPoints != expectedActionPoints {
		t.Errorf("Parsed ActionPoints = %q; want %q", dev.ActionPoints, expectedActionPoints)
	}
}
