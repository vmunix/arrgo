// internal/handlers/quality_test.go
package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vmunix/arrgo/internal/library"
)

func TestResolutionRank(t *testing.T) {
	tests := []struct {
		quality string
		rank    int
	}{
		// 4K variants
		{"2160p", 4},
		{"4k", 4},
		{"4K", 4},
		{"uhd", 4},
		{"UHD", 4},
		// 1080p variants
		{"1080p", 3},
		{"fhd", 3},
		{"FHD", 3},
		// 720p variants
		{"720p", 2},
		{"hd", 2},
		{"HD", 2},
		// 480p variants
		{"480p", 1},
		{"sd", 1},
		{"SD", 1},
		// Unknown
		{"", 0},
		{"unknown", 0},
		{"garbage", 0},
	}

	for _, tt := range tests {
		t.Run(tt.quality, func(t *testing.T) {
			assert.Equal(t, tt.rank, resolutionRank(tt.quality))
		})
	}
}

func TestIsBetterQuality(t *testing.T) {
	tests := []struct {
		name     string
		new      string
		existing string
		better   bool
	}{
		// Upgrades
		{"4K beats 1080p", "2160p", "1080p", true},
		{"4K beats 720p", "2160p", "720p", true},
		{"1080p beats 720p", "1080p", "720p", true},
		{"720p beats 480p", "720p", "480p", true},
		// Equal quality (not better)
		{"1080p equals 1080p", "1080p", "1080p", false},
		{"4K equals 4K", "2160p", "2160p", false},
		// Downgrades (not better)
		{"1080p doesn't beat 4K", "1080p", "2160p", false},
		{"720p doesn't beat 1080p", "720p", "1080p", false},
		{"480p doesn't beat 720p", "480p", "720p", false},
		// Unknown quality
		{"unknown doesn't beat 1080p", "unknown", "1080p", false},
		{"1080p beats unknown", "1080p", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.better, isBetterQuality(tt.new, tt.existing))
		})
	}
}

func TestGetBestQuality(t *testing.T) {
	tests := []struct {
		name     string
		files    []*library.File
		expected string
	}{
		{
			name:     "empty list",
			files:    []*library.File{},
			expected: "",
		},
		{
			name: "single file",
			files: []*library.File{
				{Quality: "1080p"},
			},
			expected: "1080p",
		},
		{
			name: "best is first",
			files: []*library.File{
				{Quality: "2160p"},
				{Quality: "1080p"},
				{Quality: "720p"},
			},
			expected: "2160p",
		},
		{
			name: "best is last",
			files: []*library.File{
				{Quality: "720p"},
				{Quality: "1080p"},
				{Quality: "2160p"},
			},
			expected: "2160p",
		},
		{
			name: "best is middle",
			files: []*library.File{
				{Quality: "720p"},
				{Quality: "2160p"},
				{Quality: "1080p"},
			},
			expected: "2160p",
		},
		{
			name: "all same quality",
			files: []*library.File{
				{Quality: "1080p"},
				{Quality: "1080p"},
			},
			expected: "1080p",
		},
		{
			name: "mixed with unknown",
			files: []*library.File{
				{Quality: "unknown"},
				{Quality: "720p"},
				{Quality: ""},
			},
			expected: "720p",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, getBestQuality(tt.files))
		})
	}
}
