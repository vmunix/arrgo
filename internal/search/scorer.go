package search

import (
	"strings"

	"github.com/arrgo/arrgo/pkg/release"
)

// QualitySpec represents one entry from a quality profile's accept list.
// It specifies a resolution and optionally a source type.
type QualitySpec struct {
	Resolution release.Resolution
	Source     release.Source // SourceUnknown means "any source"
}

// ParseQualitySpec parses strings like "1080p bluray" or "720p" into a QualitySpec.
// Examples:
//   - "1080p bluray" -> {Resolution1080p, SourceBluRay}
//   - "720p" -> {Resolution720p, SourceUnknown} (any source matches)
func ParseQualitySpec(s string) QualitySpec {
	spec := QualitySpec{}
	s = strings.ToLower(strings.TrimSpace(s))

	// Parse resolution
	switch {
	case strings.Contains(s, "2160p"):
		spec.Resolution = release.Resolution2160p
	case strings.Contains(s, "1080p"):
		spec.Resolution = release.Resolution1080p
	case strings.Contains(s, "720p"):
		spec.Resolution = release.Resolution720p
	default:
		spec.Resolution = release.ResolutionUnknown
	}

	// Parse source
	switch {
	case strings.Contains(s, "bluray"):
		spec.Source = release.SourceBluRay
	case strings.Contains(s, "webdl"):
		spec.Source = release.SourceWEBDL
	case strings.Contains(s, "webrip"):
		spec.Source = release.SourceWEBRip
	case strings.Contains(s, "hdtv"):
		spec.Source = release.SourceHDTV
	default:
		spec.Source = release.SourceUnknown
	}

	return spec
}

// Matches returns true if the given release info matches this quality spec.
// Resolution must match exactly. If spec.Source is SourceUnknown, any source matches.
// Otherwise, source must match exactly.
func (q QualitySpec) Matches(info release.Info) bool {
	// Resolution must match exactly
	if q.Resolution != info.Resolution {
		return false
	}

	// If spec source is Unknown, any source matches
	if q.Source == release.SourceUnknown {
		return true
	}

	// Otherwise, source must match exactly
	return q.Source == info.Source
}

// Scorer scores releases against quality profiles.
type Scorer struct {
	profiles map[string][]QualitySpec
}

// NewScorer creates a new Scorer from config profiles.
// Profiles is a map where key is profile name and value is an ordered list
// of quality specs (first entry is highest priority).
func NewScorer(profiles map[string][]string) *Scorer {
	s := &Scorer{
		profiles: make(map[string][]QualitySpec),
	}

	for name, specs := range profiles {
		parsed := make([]QualitySpec, len(specs))
		for i, spec := range specs {
			parsed[i] = ParseQualitySpec(spec)
		}
		s.profiles[name] = parsed
	}

	return s
}

// Score returns the quality score for a release in the given profile.
// Returns 0 if no match (release should be filtered out).
// Returns len(specs) - i where i is the index of first matching spec.
// First entry in accept list gets highest score.
func (s *Scorer) Score(info release.Info, profile string) int {
	specs, ok := s.profiles[profile]
	if !ok {
		return 0
	}

	for i, spec := range specs {
		if spec.Matches(info) {
			return len(specs) - i
		}
	}

	return 0
}
