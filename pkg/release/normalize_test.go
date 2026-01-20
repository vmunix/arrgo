package release

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
