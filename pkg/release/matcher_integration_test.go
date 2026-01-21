package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMatchTitleRealWorldCases tests fuzzy matching with real-world variations.
func TestMatchTitleRealWorldCases(t *testing.T) {
	// Simulated library titles
	library := []string{
		"Back to the Future",
		"Back to the Future Part II",
		"Back to the Future Part III",
		"The Matrix",
		"The Matrix Reloaded",
		"The Matrix Revolutions",
		"Fast & Furious",
		"Fast Five",
		"The Fast and the Furious",
		"Rocky",
		"Rocky II",
		"Rocky III",
		"Rocky IV",
	}

	tests := []struct {
		releaseName   string
		expectedTitle string
		minConfidence MatchConfidence
	}{
		// Roman numeral variations
		{"Back.to.the.Future.Part.II.1989.1080p.BluRay", "Back to the Future Part II", ConfidenceMedium},
		{"Back.to.the.Future.2.1989.1080p.BluRay", "Back to the Future Part II", ConfidenceMedium},
		{"Rocky.III.1982.2160p.UHD.BluRay", "Rocky III", ConfidenceHigh},
		{"Rocky.3.1982.2160p.UHD.BluRay", "Rocky III", ConfidenceMedium},

		// Conjunction variations
		{"Fast.and.Furious.2009.1080p.BluRay", "Fast & Furious", ConfidenceHigh},

		// Standard matches
		{"The.Matrix.1999.2160p.UHD.BluRay", "The Matrix", ConfidenceHigh},
	}

	for _, tt := range tests {
		t.Run(tt.releaseName, func(t *testing.T) {
			// Parse the release to extract title
			info := Parse(tt.releaseName)

			// Match against library
			result := MatchTitle(info.Title, library)

			assert.Equal(t, tt.expectedTitle, result.Title, "wrong title matched")
			assert.GreaterOrEqual(t, result.Confidence, tt.minConfidence, "confidence too low")
		})
	}
}
