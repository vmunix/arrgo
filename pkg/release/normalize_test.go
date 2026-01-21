package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeRomanNumerals(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Rocky III", "Rocky 3"},
		{"Part II", "Part 2"},
		{"Chapter IV", "Chapter 4"},
		{"Star Wars Episode V", "Star Wars Episode 5"},
		{"Final Fantasy VII", "Final Fantasy 7"},
		{"Resident Evil VIII", "Resident Evil 8"},
		{"Henry V", "Henry 5"},
		{"The Godfather Part II", "The Godfather Part 2"},
		// Should NOT convert
		{"I Robot", "I Robot"},                           // "I" alone is ambiguous
		{"VII Days", "VII Days"},                         // Roman at start without context
		{"Matrix", "Matrix"},                             // No roman numerals
		{"2001 A Space Odyssey", "2001 A Space Odyssey"}, // Arabic stays
		// X is excluded to avoid false positives
		{"Malcolm X", "Malcolm X"},
		{"American History X", "American History X"},
		{"Rocky X", "Rocky X"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeRomanNumerals(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"The Matrix", "matrix"},
		{"A Beautiful Mind", "beautiful mind"},
		{"An American Werewolf", "american werewolf"},
		{"Fast & Furious", "fast and furious"},
		{"Léon: The Professional", "leon professional"},
		{"Spider-Man: No Way Home", "spider man no way home"},
		{"  Extra   Spaces  ", "extra spaces"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := CleanTitle(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCleanTitleWithRomanNumerals(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Back to the Future Part II", "back to the future part 2"},
		{"Rocky III", "rocky 3"},
		{"The Godfather Part III", "godfather part 3"},
		{"Fast & Furious", "fast and furious"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := CleanTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeSearchQuery(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Deadpool & Wolverine 2024", "Deadpool and Wolverine 2024"},
		{"Fast & Furious", "Fast and Furious"},
		{"Simon & Garfunkel", "Simon and Garfunkel"},
		{"  Extra   Spaces  ", "Extra Spaces"},
		{"Normal Title 2024", "Normal Title 2024"},
		{"Tom & Jerry", "Tom and Jerry"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeSearchQuery(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}
