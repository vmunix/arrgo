package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/library"
)

func TestMatchFileToEpisode(t *testing.T) {
	episodes := []*library.Episode{
		{ID: 1, Season: 1, Episode: 1},
		{ID: 2, Season: 1, Episode: 2},
		{ID: 3, Season: 1, Episode: 3},
	}

	tests := []struct {
		name     string
		filename string
		wantID   int64
		wantErr  bool
	}{
		{
			name:     "standard format",
			filename: "Show.S01E02.1080p.mkv",
			wantID:   2,
		},
		{
			name:     "lowercase",
			filename: "show.s01e01.720p.mkv",
			wantID:   1,
		},
		{
			name:     "no match",
			filename: "Show.S01E05.1080p.mkv",
			wantErr:  true,
		},
		{
			name:     "multi-episode returns first",
			filename: "Show.S01E01E02.1080p.mkv",
			wantID:   1,
		},
		{
			name:     "unparseable",
			filename: "random_file.mkv",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep, err := MatchFileToEpisode(tt.filename, episodes)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, ep.ID)
		})
	}
}

func TestMatchFilesToEpisodes(t *testing.T) {
	episodes := []*library.Episode{
		{ID: 1, Season: 1, Episode: 1},
		{ID: 2, Season: 1, Episode: 2},
		{ID: 3, Season: 1, Episode: 3},
	}

	files := []string{
		"/downloads/Show.S01/Show.S01E01.mkv",
		"/downloads/Show.S01/Show.S01E02.mkv",
		"/downloads/Show.S01/Show.S01E03.mkv",
		"/downloads/Show.S01/sample.mkv", // Should be unmatched
	}

	matches, unmatched := MatchFilesToEpisodes(files, episodes)

	assert.Len(t, matches, 3)
	assert.Len(t, unmatched, 1)
	assert.Equal(t, "/downloads/Show.S01/sample.mkv", unmatched[0])

	// Verify correct matching
	for _, m := range matches {
		assert.NotNil(t, m.Episode)
	}
}

func TestMatchFileToSeason(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		wantSeason int
		wantEp     int
		wantErr    bool
	}{
		{
			name:       "standard",
			filename:   "Show.S02E05.1080p.mkv",
			wantSeason: 2,
			wantEp:     5,
		},
		{
			name:       "multi-episode",
			filename:   "Show.S01E05E06.mkv",
			wantSeason: 1,
			wantEp:     5, // Returns first episode
		},
		{
			name:     "no episode",
			filename: "Show.S01.1080p.mkv", // Season pack folder, no episode
			wantErr:  true,
		},
		{
			name:     "unparseable",
			filename: "random.mkv",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			season, ep, err := MatchFileToSeason(tt.filename)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSeason, season)
			assert.Equal(t, tt.wantEp, ep)
		})
	}
}
