// internal/importer/renamer.go
package importer

import (
	"fmt"
	"regexp"
	"strconv"
)

// Default naming templates.
const (
	DefaultMovieTemplate  = "{title} ({year})/{title} ({year}) - {quality}.{ext}"
	DefaultSeriesTemplate = "{title}/Season {season:02}/{title} - S{season:02}E{episode:02} - {quality}.{ext}"
)

// Renamer applies naming templates to generate file paths.
type Renamer struct {
	movieTemplate  string
	seriesTemplate string
}

// NewRenamer creates a new Renamer with the given templates.
// Empty strings use default templates.
func NewRenamer(movieTemplate, seriesTemplate string) *Renamer {
	if movieTemplate == "" {
		movieTemplate = DefaultMovieTemplate
	}
	if seriesTemplate == "" {
		seriesTemplate = DefaultSeriesTemplate
	}
	return &Renamer{
		movieTemplate:  movieTemplate,
		seriesTemplate: seriesTemplate,
	}
}

// MoviePath generates the relative path for a movie file.
func (r *Renamer) MoviePath(title string, year int, quality, ext string) string {
	title = SanitizeFilename(title)
	vars := map[string]any{
		"title":   title,
		"year":    year,
		"quality": quality,
		"ext":     ext,
	}
	return applyTemplate(r.movieTemplate, vars)
}

// EpisodePath generates the relative path for an episode file.
func (r *Renamer) EpisodePath(title string, season, episode int, quality, ext string) string {
	title = SanitizeFilename(title)
	vars := map[string]any{
		"title":   title,
		"season":  season,
		"episode": episode,
		"quality": quality,
		"ext":     ext,
	}
	return applyTemplate(r.seriesTemplate, vars)
}

// formatPattern matches {name} or {name:02} style placeholders.
var formatPattern = regexp.MustCompile(`\{(\w+)(?::(\d+))?\}`)

// applyTemplate substitutes variables into a template string.
// Supports {name} for simple substitution and {name:02} for zero-padded integers.
func applyTemplate(template string, vars map[string]any) string {
	return formatPattern.ReplaceAllStringFunc(template, func(match string) string {
		parts := formatPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}

		name := parts[1]
		val, ok := vars[name]
		if !ok {
			return match
		}

		// Check for format specifier (e.g., :02)
		if len(parts) >= 3 && parts[2] != "" {
			width, err := strconv.Atoi(parts[2])
			if err == nil {
				// Zero-pad integer values
				switch v := val.(type) {
				case int:
					return fmt.Sprintf("%0*d", width, v)
				case int64:
					return fmt.Sprintf("%0*d", width, v)
				}
			}
		}

		// Simple string conversion
		return fmt.Sprintf("%v", val)
	})
}
