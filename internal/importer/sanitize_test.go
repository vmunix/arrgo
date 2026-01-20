// internal/importer/sanitize_test.go
package importer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "Movie Name", "Movie Name"},
		{"path separators", "Movie/Name\\Here", "Movie Name Here"},
		{"path traversal", "../../../etc/passwd", "etc passwd"},
		{"double dots", "Movie..Name", "Movie.Name"},
		{"illegal chars", "Movie: The *Best* <One>", "Movie The Best One"},
		{"null bytes", "Movie\x00Name", "MovieName"},
		{"multiple spaces", "Movie   Name", "Movie Name"},
		{"leading/trailing", "  .Movie Name.  ", "Movie Name"},
		{"question mark", "What?", "What"},
		{"pipe", "This|That", "This That"},
		{"quotes", `Movie "Name"`, "Movie Name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			assert.Equal(t, tt.want, got, "SanitizeFilename(%q)", tt.input)
		})
	}
}

func TestValidatePath(t *testing.T) {
	root := "/movies"

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid subpath", "/movies/Movie (2024)/movie.mkv", false},
		{"valid nested", "/movies/A/B/C/movie.mkv", false},
		{"exact root", "/movies", false},
		{"traversal attempt", "/movies/../etc/passwd", true},
		{"outside root", "/tv/show.mkv", true},
		{"sneaky traversal", "/movies/foo/../../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePath(tt.path, root)
			if tt.wantErr {
				assert.Error(t, err, "ValidatePath(%q, %q)", tt.path, root)
			} else {
				assert.NoError(t, err, "ValidatePath(%q, %q)", tt.path, root)
			}
		})
	}
}

func TestIsVideoFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"movie.mkv", true},
		{"movie.MKV", true},
		{"movie.mp4", true},
		{"movie.avi", true},
		{"movie.m4v", true},
		{"movie.txt", false},
		{"movie.nfo", false},
		{"movie.srt", false},
		{".mkv", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, IsVideoFile(tt.path), "IsVideoFile(%q)", tt.path)
		})
	}
}
