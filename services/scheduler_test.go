package services

import (
	"math/rand"
	"reflect"
	"testing"
)

func TestIsLeapYear(t *testing.T) {
	tests := []struct {
		year     int
		expected bool
	}{
		{2000, true},
		{2004, true},
		{2100, false},
		{2023, false},
		{2026, false},
		{2400, true},
	}

	for _, tt := range tests {
		result := isLeapYear(tt.year)
		if result != tt.expected {
			t.Errorf("isLeapYear(%d) = %v; want %v", tt.year, result, tt.expected)
		}
	}
}

func TestDeterministicShuffling(t *testing.T) {
	// Verify that shuffling is deterministic for the same year but different across years
	items1 := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	items2 := make([]string, len(items1))
	copy(items2, items1)

	// Shuffle with seed for 2026
	r1 := rand.New(rand.NewSource(int64(2026)))
	r1.Shuffle(len(items1), func(i, j int) {
		items1[i], items1[j] = items1[j], items1[i]
	})

	r2 := rand.New(rand.NewSource(int64(2026)))
	r2.Shuffle(len(items2), func(i, j int) {
		items2[i], items2[j] = items2[j], items2[i]
	})

	if !reflect.DeepEqual(items1, items2) {
		t.Errorf("Shuffling with same seed was not deterministic")
	}

	// Shuffle with seed for 2027
	items3 := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	r3 := rand.New(rand.NewSource(int64(2027)))
	r3.Shuffle(len(items3), func(i, j int) {
		items3[i], items3[j] = items3[j], items3[i]
	})

	if reflect.DeepEqual(items1, items3) {
		t.Errorf("Shuffling was same for year 2026 and year 2027")
	}
}
