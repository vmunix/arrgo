// internal/importer/renamer_test.go
package importer

import "testing"

func TestRenamer_MoviePath(t *testing.T) {
	r := NewRenamer("", "") // Use defaults

	tests := []struct {
		name    string
		title   string
		year    int
		quality string
		ext     string
		want    string
	}{
		{
			name:    "basic movie",
			title:   "The Matrix",
			year:    1999,
			quality: "1080p",
			ext:     "mkv",
			want:    "The Matrix (1999)/The Matrix (1999) - 1080p.mkv",
		},
		{
			name:    "movie with special chars",
			title:   "What If...?",
			year:    2024,
			quality: "720p",
			ext:     "mp4",
			want:    "What If (2024)/What If (2024) - 720p.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.MoviePath(tt.title, tt.year, tt.quality, tt.ext)
			if got != tt.want {
				t.Errorf("MoviePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenamer_EpisodePath(t *testing.T) {
	r := NewRenamer("", "") // Use defaults

	tests := []struct {
		name    string
		title   string
		season  int
		episode int
		quality string
		ext     string
		want    string
	}{
		{
			name:    "basic episode",
			title:   "Breaking Bad",
			season:  1,
			episode: 5,
			quality: "1080p",
			ext:     "mkv",
			want:    "Breaking Bad/Season 01/Breaking Bad - S01E05 - 1080p.mkv",
		},
		{
			name:    "double digit season",
			title:   "Supernatural",
			season:  15,
			episode: 20,
			quality: "720p",
			ext:     "mp4",
			want:    "Supernatural/Season 15/Supernatural - S15E20 - 720p.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.EpisodePath(tt.title, tt.season, tt.episode, tt.quality, tt.ext)
			if got != tt.want {
				t.Errorf("EpisodePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenamer_CustomTemplate(t *testing.T) {
	r := NewRenamer(
		"{title}/{title}.{ext}",
		"{title}/S{season:02}E{episode:02}.{ext}",
	)

	moviePath := r.MoviePath("Movie", 2024, "1080p", "mkv")
	if moviePath != "Movie/Movie.mkv" {
		t.Errorf("custom movie template: got %q", moviePath)
	}

	episodePath := r.EpisodePath("Show", 1, 5, "720p", "mkv")
	if episodePath != "Show/S01E05.mkv" {
		t.Errorf("custom episode template: got %q", episodePath)
	}
}

func TestApplyTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     map[string]any
		want     string
	}{
		{
			name:     "simple substitution",
			template: "{title} ({year})",
			vars:     map[string]any{"title": "Movie", "year": 2024},
			want:     "Movie (2024)",
		},
		{
			name:     "zero padding",
			template: "S{season:02}E{episode:02}",
			vars:     map[string]any{"season": 1, "episode": 5},
			want:     "S01E05",
		},
		{
			name:     "three digit padding",
			template: "E{episode:03}",
			vars:     map[string]any{"episode": 7},
			want:     "E007",
		},
		{
			name:     "no padding needed",
			template: "S{season:02}",
			vars:     map[string]any{"season": 12},
			want:     "S12",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyTemplate(tt.template, tt.vars)
			if got != tt.want {
				t.Errorf("applyTemplate() = %q, want %q", got, tt.want)
			}
		})
	}
}
