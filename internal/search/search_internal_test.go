package search

import "testing"

func TestHasSequelMismatch(t *testing.T) {
	tests := []struct {
		query    string
		title    string
		mismatch bool
	}{
		// No sequel in either - no mismatch
		{"Back to the Future", "Back to the Future", false},
		// Sequel in title but not query - mismatch
		{"Back to the Future", "Back to the Future Part II", true},
		{"Back to the Future", "Back to the Future Part III", true},
		{"Back to the Future", "Back.to.the.Future.II.1989", true}, // Roman numeral only
		{"The Matrix", "The.Matrix.III.2003", true},                // Roman numeral only
		// Sequel in both - matching numbers - no mismatch
		{"Back to the Future Part II", "Back to the Future Part II", false},
		{"Back to the Future Part 2", "Back to the Future Part II", false}, // Part 2 matches II
		{"Back to the Future Part 2", "Back.to.the.Future.II.1989", false}, // Part 2 matches II
		// Sequel in both - different numbers - mismatch
		{"Back to the Future Part 2", "Back to the Future Part III", true},  // Part 2 != III
		{"Back to the Future Part II", "Back.to.the.Future.III.1990", true}, // II != III
		// Query has sequel, title doesn't - no mismatch (original is fine)
		{"Back to the Future Part II", "Back to the Future", false},
		// Audio specs should NOT trigger sequel detection
		{"Back to the Future", "Back.to.the.Future.1985.DD.5.1", false},
	}

	for _, tc := range tests {
		got := hasSequelMismatch(tc.query, tc.title)
		if got != tc.mismatch {
			t.Errorf("hasSequelMismatch(%q, %q) = %v, want %v", tc.query, tc.title, got, tc.mismatch)
		}
	}
}
