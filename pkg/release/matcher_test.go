package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchConfidenceString(t *testing.T) {
	tests := []struct {
		conf     MatchConfidence
		expected string
	}{
		{ConfidenceHigh, "high"},
		{ConfidenceMedium, "medium"},
		{ConfidenceLow, "low"},
		{ConfidenceNone, "none"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.conf.String())
		})
	}
}

func TestMatchTitle(t *testing.T) {
	candidates := []string{
		"The Matrix",
		"The Matrix Reloaded",
		"The Matrix Revolutions",
		"Back to the Future",
		"Back to the Future Part II",
		"Back to the Future Part III",
	}

	tests := []struct {
		name          string
		parsed        string
		expectedTitle string
		expectedConf  MatchConfidence
		minScore      float64
	}{
		{
			name:          "exact match",
			parsed:        "The Matrix",
			expectedTitle: "The Matrix",
			expectedConf:  ConfidenceHigh,
			minScore:      0.99,
		},
		{
			name:          "close match with roman numeral",
			parsed:        "Back to the Future 2",
			expectedTitle: "Back to the Future Part II",
			expectedConf:  ConfidenceMedium,
			minScore:      0.80,
		},
		{
			name:          "no match",
			parsed:        "Completely Different Movie",
			expectedTitle: "",
			expectedConf:  ConfidenceNone,
			minScore:      0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MatchTitle(tt.parsed, candidates)
			if tt.expectedConf == ConfidenceNone {
				assert.Equal(t, ConfidenceNone, result.Confidence)
			} else {
				assert.Equal(t, tt.expectedTitle, result.Title)
				assert.GreaterOrEqual(t, result.Score, tt.minScore)
				assert.GreaterOrEqual(t, result.Confidence, tt.expectedConf)
			}
		})
	}
}

func TestMatchTitleEmptyCandidates(t *testing.T) {
	result := MatchTitle("Any Title", []string{})
	assert.Equal(t, ConfidenceNone, result.Confidence)
	assert.Equal(t, "", result.Title)
	assert.InDelta(t, 0.0, result.Score, 0.001)
}
