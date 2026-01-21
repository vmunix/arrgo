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
