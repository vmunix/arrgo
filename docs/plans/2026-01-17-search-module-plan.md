# Search Module Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement search module with Prowlarr integration, release name parsing, and quality scoring.

**Architecture:** Two packages: `pkg/release` for reusable release name parsing (resolution, source, codec extraction), and `internal/search` for Prowlarr HTTP client, quality scoring, and search orchestration.

**Tech Stack:** Go stdlib (net/http, regexp, encoding/json), existing config module for quality profiles.

---

### Task 1: Release Types (`pkg/release`)

**Files:**
- Create: `pkg/release/release.go`
- Create: `pkg/release/release_test.go`

**Step 1: Write the test for enum types**

```go
// pkg/release/release_test.go
package release

import "testing"

func TestResolution_String(t *testing.T) {
	tests := []struct {
		r    Resolution
		want string
	}{
		{ResolutionUnknown, "unknown"},
		{Resolution720p, "720p"},
		{Resolution1080p, "1080p"},
		{Resolution2160p, "2160p"},
	}
	for _, tt := range tests {
		if got := tt.r.String(); got != tt.want {
			t.Errorf("Resolution(%d).String() = %q, want %q", tt.r, got, tt.want)
		}
	}
}

func TestSource_String(t *testing.T) {
	tests := []struct {
		s    Source
		want string
	}{
		{SourceUnknown, "unknown"},
		{SourceBluRay, "bluray"},
		{SourceWEBDL, "webdl"},
		{SourceWEBRip, "webrip"},
		{SourceHDTV, "hdtv"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("Source(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

func TestCodec_String(t *testing.T) {
	tests := []struct {
		c    Codec
		want string
	}{
		{CodecUnknown, "unknown"},
		{CodecX264, "x264"},
		{CodecX265, "x265"},
	}
	for _, tt := range tests {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("Codec(%d).String() = %q, want %q", tt.c, got, tt.want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release/... -v`
Expected: FAIL (package doesn't exist)

**Step 3: Write minimal implementation**

```go
// pkg/release/release.go
// Package release parses release names to extract quality information.
package release

// Resolution represents video resolution.
type Resolution int

const (
	ResolutionUnknown Resolution = iota
	Resolution720p
	Resolution1080p
	Resolution2160p
)

func (r Resolution) String() string {
	switch r {
	case Resolution720p:
		return "720p"
	case Resolution1080p:
		return "1080p"
	case Resolution2160p:
		return "2160p"
	default:
		return "unknown"
	}
}

// Source represents the release source.
type Source int

const (
	SourceUnknown Source = iota
	SourceBluRay
	SourceWEBDL
	SourceWEBRip
	SourceHDTV
)

func (s Source) String() string {
	switch s {
	case SourceBluRay:
		return "bluray"
	case SourceWEBDL:
		return "webdl"
	case SourceWEBRip:
		return "webrip"
	case SourceHDTV:
		return "hdtv"
	default:
		return "unknown"
	}
}

// Codec represents the video codec.
type Codec int

const (
	CodecUnknown Codec = iota
	CodecX264
	CodecX265
)

func (c Codec) String() string {
	switch c {
	case CodecX264:
		return "x264"
	case CodecX265:
		return "x265"
	default:
		return "unknown"
	}
}

// Info contains parsed release information.
type Info struct {
	Title      string
	Resolution Resolution
	Source     Source
	Codec      Codec
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/release.go pkg/release/release_test.go
git commit -m "feat(release): add release types with Resolution, Source, Codec enums"
```

---

### Task 2: Release Parser

**Files:**
- Modify: `pkg/release/release.go`
- Modify: `pkg/release/release_test.go`

**Step 1: Write tests for Parse function**

Add to `pkg/release/release_test.go`:

```go
func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		wantRes    Resolution
		wantSource Source
		wantCodec  Codec
	}{
		// Resolution tests
		{"2160p UHD", "Movie.2024.2160p.UHD.BluRay.x265-GROUP", Resolution2160p, SourceBluRay, CodecX265},
		{"4K variant", "Movie.2024.4K.WEB-DL.x265-GROUP", Resolution2160p, SourceWEBDL, CodecX265},
		{"1080p standard", "Movie.2024.1080p.BluRay.x264-GROUP", Resolution1080p, SourceBluRay, CodecX264},
		{"720p standard", "Movie.2024.720p.HDTV.x264-GROUP", Resolution720p, SourceHDTV, CodecX264},

		// Source tests
		{"BluRay hyphen", "Movie.2024.1080p.Blu-Ray.x264-GROUP", Resolution1080p, SourceBluRay, CodecX264},
		{"BluRay no hyphen", "Movie.2024.1080p.BluRay.x264-GROUP", Resolution1080p, SourceBluRay, CodecX264},
		{"BDRip variant", "Movie.2024.1080p.BDRip.x264-GROUP", Resolution1080p, SourceBluRay, CodecX264},
		{"WEB-DL hyphen", "Movie.2024.1080p.WEB-DL.x264-GROUP", Resolution1080p, SourceWEBDL, CodecX264},
		{"WEBDL no hyphen", "Movie.2024.1080p.WEBDL.x264-GROUP", Resolution1080p, SourceWEBDL, CodecX264},
		{"WEBRip", "Movie.2024.1080p.WEBRip.x264-GROUP", Resolution1080p, SourceWEBRip, CodecX264},
		{"HDTV", "Movie.2024.720p.HDTV.x264-GROUP", Resolution720p, SourceHDTV, CodecX264},

		// Codec tests
		{"x264", "Movie.2024.1080p.BluRay.x264-GROUP", Resolution1080p, SourceBluRay, CodecX264},
		{"x265", "Movie.2024.1080p.BluRay.x265-GROUP", Resolution1080p, SourceBluRay, CodecX265},
		{"H.264", "Movie.2024.1080p.BluRay.H.264-GROUP", Resolution1080p, SourceBluRay, CodecX264},
		{"H264 no dot", "Movie.2024.1080p.BluRay.H264-GROUP", Resolution1080p, SourceBluRay, CodecX264},
		{"HEVC", "Movie.2024.1080p.BluRay.HEVC-GROUP", Resolution1080p, SourceBluRay, CodecX265},
		{"H.265", "Movie.2024.1080p.BluRay.H.265-GROUP", Resolution1080p, SourceBluRay, CodecX265},

		// Unknown values
		{"unknown resolution", "Movie.2024.BluRay.x264-GROUP", ResolutionUnknown, SourceBluRay, CodecX264},
		{"unknown source", "Movie.2024.1080p.x264-GROUP", Resolution1080p, SourceUnknown, CodecX264},
		{"unknown codec", "Movie.2024.1080p.BluRay-GROUP", Resolution1080p, SourceBluRay, CodecUnknown},
		{"all unknown", "Movie.2024-GROUP", ResolutionUnknown, SourceUnknown, CodecUnknown},

		// Real-world examples
		{"real 1", "The.Matrix.1999.2160p.UHD.BluRay.x265.10bit.HDR.DTS-HD.MA.7.1-GROUP", Resolution2160p, SourceBluRay, CodecX265},
		{"real 2", "Breaking.Bad.S01E01.1080p.BluRay.x264-DEMAND", Resolution1080p, SourceBluRay, CodecX264},
		{"real 3", "Dune.Part.Two.2024.1080p.WEB-DL.DDP5.1.Atmos.H.264-FLUX", Resolution1080p, SourceWEBDL, CodecX264},
		{"real 4", "Shogun.2024.S01E01.720p.HDTV.x264-SYNCOPY", Resolution720p, SourceHDTV, CodecX264},

		// Case insensitivity
		{"lowercase", "movie.2024.1080p.bluray.x264-group", Resolution1080p, SourceBluRay, CodecX264},
		{"UPPERCASE", "MOVIE.2024.1080P.BLURAY.X264-GROUP", Resolution1080p, SourceBluRay, CodecX264},
		{"mixed case", "Movie.2024.1080P.BluRay.X264-GROUP", Resolution1080p, SourceBluRay, CodecX264},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := Parse(tt.title)
			if info.Resolution != tt.wantRes {
				t.Errorf("Resolution = %v, want %v", info.Resolution, tt.wantRes)
			}
			if info.Source != tt.wantSource {
				t.Errorf("Source = %v, want %v", info.Source, tt.wantSource)
			}
			if info.Codec != tt.wantCodec {
				t.Errorf("Codec = %v, want %v", info.Codec, tt.wantCodec)
			}
			if info.Title != tt.title {
				t.Errorf("Title = %q, want %q", info.Title, tt.title)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/release/... -v`
Expected: FAIL (Parse undefined)

**Step 3: Write minimal implementation**

Add to `pkg/release/release.go`:

```go
import "regexp"

var (
	// Resolution patterns
	resolution2160p = regexp.MustCompile(`(?i)\b(2160p|4k)\b`)
	resolution1080p = regexp.MustCompile(`(?i)\b1080p\b`)
	resolution720p  = regexp.MustCompile(`(?i)\b720p\b`)

	// Source patterns (order matters - more specific first)
	sourceBluRay = regexp.MustCompile(`(?i)\b(blu-?ray|bdrip|bdremux)\b`)
	sourceWEBDL  = regexp.MustCompile(`(?i)\b(web-?dl|webdl)\b`)
	sourceWEBRip = regexp.MustCompile(`(?i)\bwebrip\b`)
	sourceHDTV   = regexp.MustCompile(`(?i)\bhdtv\b`)

	// Codec patterns
	codecX265 = regexp.MustCompile(`(?i)\b(x\.?265|h\.?265|hevc)\b`)
	codecX264 = regexp.MustCompile(`(?i)\b(x\.?264|h\.?264)\b`)
)

// Parse extracts quality information from a release name.
func Parse(title string) Info {
	info := Info{Title: title}

	// Resolution
	switch {
	case resolution2160p.MatchString(title):
		info.Resolution = Resolution2160p
	case resolution1080p.MatchString(title):
		info.Resolution = Resolution1080p
	case resolution720p.MatchString(title):
		info.Resolution = Resolution720p
	default:
		info.Resolution = ResolutionUnknown
	}

	// Source
	switch {
	case sourceBluRay.MatchString(title):
		info.Source = SourceBluRay
	case sourceWEBDL.MatchString(title):
		info.Source = SourceWEBDL
	case sourceWEBRip.MatchString(title):
		info.Source = SourceWEBRip
	case sourceHDTV.MatchString(title):
		info.Source = SourceHDTV
	default:
		info.Source = SourceUnknown
	}

	// Codec (x265 first since it's more specific than x264)
	switch {
	case codecX265.MatchString(title):
		info.Codec = CodecX265
	case codecX264.MatchString(title):
		info.Codec = CodecX264
	default:
		info.Codec = CodecUnknown
	}

	return info
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./pkg/release/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add pkg/release/release.go pkg/release/release_test.go
git commit -m "feat(release): implement Parse function with regex patterns"
```

---

### Task 3: Search Error Types

**Files:**
- Create: `internal/search/errors.go`
- Create: `internal/search/errors_test.go`

**Step 1: Write the test**

```go
// internal/search/errors_test.go
package search

import (
	"errors"
	"testing"
)

func TestErrors(t *testing.T) {
	// Verify errors are distinct
	if errors.Is(ErrProwlarrUnavailable, ErrInvalidAPIKey) {
		t.Error("ErrProwlarrUnavailable should not equal ErrInvalidAPIKey")
	}
	if errors.Is(ErrInvalidAPIKey, ErrNoResults) {
		t.Error("ErrInvalidAPIKey should not equal ErrNoResults")
	}

	// Verify error messages
	if ErrProwlarrUnavailable.Error() == "" {
		t.Error("ErrProwlarrUnavailable should have a message")
	}
	if ErrInvalidAPIKey.Error() == "" {
		t.Error("ErrInvalidAPIKey should have a message")
	}
	if ErrNoResults.Error() == "" {
		t.Error("ErrNoResults should have a message")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/search/... -v -run TestErrors`
Expected: FAIL (errors undefined)

**Step 3: Write minimal implementation**

```go
// internal/search/errors.go
package search

import "errors"

var (
	// ErrProwlarrUnavailable indicates Prowlarr could not be reached.
	ErrProwlarrUnavailable = errors.New("prowlarr unavailable")

	// ErrInvalidAPIKey indicates the Prowlarr API key is invalid.
	ErrInvalidAPIKey = errors.New("invalid prowlarr api key")

	// ErrNoResults indicates no matching releases were found.
	// This is informational, not a failure.
	ErrNoResults = errors.New("no matching releases found")
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/search/... -v -run TestErrors`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/search/errors.go internal/search/errors_test.go
git commit -m "feat(search): add error types"
```

---

### Task 4: Quality Scorer

**Files:**
- Create: `internal/search/scorer.go`
- Create: `internal/search/scorer_test.go`

**Step 1: Write the test**

```go
// internal/search/scorer_test.go
package search

import (
	"testing"

	"github.com/user/arrgo/pkg/release"
)

func TestParseQualitySpec(t *testing.T) {
	tests := []struct {
		input   string
		wantRes release.Resolution
		wantSrc release.Source
	}{
		{"1080p bluray", release.Resolution1080p, release.SourceBluRay},
		{"1080p webdl", release.Resolution1080p, release.SourceWEBDL},
		{"720p", release.Resolution720p, release.SourceUnknown},
		{"2160p bluray", release.Resolution2160p, release.SourceBluRay},
		{"1080p hdtv", release.Resolution1080p, release.SourceHDTV},
		{"1080p webrip", release.Resolution1080p, release.SourceWEBRip},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			spec := ParseQualitySpec(tt.input)
			if spec.Resolution != tt.wantRes {
				t.Errorf("Resolution = %v, want %v", spec.Resolution, tt.wantRes)
			}
			if spec.Source != tt.wantSrc {
				t.Errorf("Source = %v, want %v", spec.Source, tt.wantSrc)
			}
		})
	}
}

func TestQualitySpec_Matches(t *testing.T) {
	tests := []struct {
		name  string
		spec  QualitySpec
		info  release.Info
		match bool
	}{
		{
			"exact match",
			QualitySpec{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			release.Info{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			true,
		},
		{
			"resolution only",
			QualitySpec{Resolution: release.Resolution1080p, Source: release.SourceUnknown},
			release.Info{Resolution: release.Resolution1080p, Source: release.SourceWEBDL},
			true,
		},
		{
			"wrong resolution",
			QualitySpec{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			release.Info{Resolution: release.Resolution720p, Source: release.SourceBluRay},
			false,
		},
		{
			"wrong source",
			QualitySpec{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			release.Info{Resolution: release.Resolution1080p, Source: release.SourceHDTV},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.Matches(tt.info); got != tt.match {
				t.Errorf("Matches() = %v, want %v", got, tt.match)
			}
		})
	}
}

func TestScorer_Score(t *testing.T) {
	profiles := map[string][]string{
		"hd": {"1080p bluray", "1080p webdl", "1080p hdtv", "720p bluray"},
	}
	scorer := NewScorer(profiles)

	tests := []struct {
		name    string
		info    release.Info
		profile string
		want    int
	}{
		{
			"first choice (highest score)",
			release.Info{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			"hd",
			4, // len(4) - 0
		},
		{
			"second choice",
			release.Info{Resolution: release.Resolution1080p, Source: release.SourceWEBDL},
			"hd",
			3, // len(4) - 1
		},
		{
			"last choice",
			release.Info{Resolution: release.Resolution720p, Source: release.SourceBluRay},
			"hd",
			1, // len(4) - 3
		},
		{
			"no match",
			release.Info{Resolution: release.Resolution720p, Source: release.SourceHDTV},
			"hd",
			0,
		},
		{
			"unknown profile",
			release.Info{Resolution: release.Resolution1080p, Source: release.SourceBluRay},
			"nonexistent",
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scorer.Score(tt.info, tt.profile); got != tt.want {
				t.Errorf("Score() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/search/... -v -run "TestParseQualitySpec|TestQualitySpec_Matches|TestScorer"`
Expected: FAIL (types undefined)

**Step 3: Write minimal implementation**

```go
// internal/search/scorer.go
package search

import (
	"strings"

	"github.com/user/arrgo/pkg/release"
)

// QualitySpec represents a single quality specification from a profile.
type QualitySpec struct {
	Resolution release.Resolution
	Source     release.Source
}

// ParseQualitySpec parses a quality spec string like "1080p bluray".
func ParseQualitySpec(s string) QualitySpec {
	s = strings.ToLower(s)
	spec := QualitySpec{}

	// Resolution
	switch {
	case strings.Contains(s, "2160p") || strings.Contains(s, "4k"):
		spec.Resolution = release.Resolution2160p
	case strings.Contains(s, "1080p"):
		spec.Resolution = release.Resolution1080p
	case strings.Contains(s, "720p"):
		spec.Resolution = release.Resolution720p
	}

	// Source
	switch {
	case strings.Contains(s, "bluray"):
		spec.Source = release.SourceBluRay
	case strings.Contains(s, "webdl"):
		spec.Source = release.SourceWEBDL
	case strings.Contains(s, "webrip"):
		spec.Source = release.SourceWEBRip
	case strings.Contains(s, "hdtv"):
		spec.Source = release.SourceHDTV
	}

	return spec
}

// Matches returns true if the release info matches this spec.
// If Source is Unknown, any source matches.
func (q QualitySpec) Matches(info release.Info) bool {
	if info.Resolution != q.Resolution {
		return false
	}
	if q.Source != release.SourceUnknown && info.Source != q.Source {
		return false
	}
	return true
}

// Scorer scores releases against quality profiles.
type Scorer struct {
	profiles map[string][]QualitySpec
}

// NewScorer creates a scorer from profile configuration.
// The map key is profile name, value is list of accept specs in priority order.
func NewScorer(profiles map[string][]string) *Scorer {
	s := &Scorer{
		profiles: make(map[string][]QualitySpec),
	}
	for name, accepts := range profiles {
		specs := make([]QualitySpec, len(accepts))
		for i, accept := range accepts {
			specs[i] = ParseQualitySpec(accept)
		}
		s.profiles[name] = specs
	}
	return s
}

// Score returns the match score for a release against a profile.
// Higher scores are better. Returns 0 if no match (release should be filtered out).
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
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/search/... -v -run "TestParseQualitySpec|TestQualitySpec_Matches|TestScorer"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/search/scorer.go internal/search/scorer_test.go
git commit -m "feat(search): implement quality scorer with profile matching"
```

---

### Task 5: Prowlarr Client

**Files:**
- Create: `internal/search/prowlarr.go`
- Create: `internal/search/prowlarr_test.go`

**Step 1: Write the test**

```go
// internal/search/prowlarr_test.go
package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProwlarrClient_Search(t *testing.T) {
	// Mock Prowlarr response
	mockReleases := []prowlarrRelease{
		{
			Title:       "Movie.2024.1080p.BluRay.x264-GROUP",
			GUID:        "abc123",
			Indexer:     "TestIndexer",
			DownloadURL: "http://example.com/download/abc123",
			Size:        5000000000,
			PublishDate: "2024-01-15T12:00:00Z",
		},
		{
			Title:       "Movie.2024.720p.HDTV.x264-OTHER",
			GUID:        "def456",
			Indexer:     "TestIndexer",
			DownloadURL: "http://example.com/download/def456",
			Size:        2000000000,
			PublishDate: "2024-01-14T10:00:00Z",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify API key header
		if r.Header.Get("X-Api-Key") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Verify path
		if r.URL.Path != "/api/v1/search" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockReleases)
	}))
	defer server.Close()

	client := NewProwlarrClient(server.URL, "test-key")
	releases, err := client.Search(context.Background(), Query{Text: "Movie 2024", Type: "movie"})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}

	if releases[0].Title != "Movie.2024.1080p.BluRay.x264-GROUP" {
		t.Errorf("unexpected title: %s", releases[0].Title)
	}
	if releases[0].GUID != "abc123" {
		t.Errorf("unexpected GUID: %s", releases[0].GUID)
	}
}

func TestProwlarrClient_Search_InvalidAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewProwlarrClient(server.URL, "bad-key")
	_, err := client.Search(context.Background(), Query{Text: "test"})
	if err == nil {
		t.Error("expected error for invalid API key")
	}
}

func TestProwlarrClient_Search_Unavailable(t *testing.T) {
	client := NewProwlarrClient("http://localhost:99999", "key")
	_, err := client.Search(context.Background(), Query{Text: "test"})
	if err == nil {
		t.Error("expected error for unavailable server")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/search/... -v -run TestProwlarrClient`
Expected: FAIL (types undefined)

**Step 3: Write minimal implementation**

```go
// internal/search/prowlarr.go
package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// prowlarrRelease is the JSON response from Prowlarr API.
type prowlarrRelease struct {
	Title       string `json:"title"`
	GUID        string `json:"guid"`
	Indexer     string `json:"indexer"`
	DownloadURL string `json:"downloadUrl"`
	Size        int64  `json:"size"`
	PublishDate string `json:"publishDate"`
}

// ProwlarrRelease is a release from Prowlarr with parsed publish date.
type ProwlarrRelease struct {
	Title       string
	GUID        string
	Indexer     string
	DownloadURL string
	Size        int64
	PublishDate time.Time
}

// ProwlarrClient is an HTTP client for the Prowlarr API.
type ProwlarrClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewProwlarrClient creates a new Prowlarr client.
func NewProwlarrClient(baseURL, apiKey string) *ProwlarrClient {
	return &ProwlarrClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Search queries Prowlarr for releases matching the query.
func (c *ProwlarrClient) Search(ctx context.Context, q Query) ([]ProwlarrRelease, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/search")
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	params := url.Values{}
	if q.Text != "" {
		params.Set("query", q.Text)
	}
	if q.Type == "movie" {
		params.Set("categories", "2000") // Movies category
	} else if q.Type == "series" {
		params.Set("categories", "5000") // TV category
	}
	if q.TMDBID != nil {
		params.Set("tmdbId", strconv.FormatInt(*q.TMDBID, 10))
	}
	if q.TVDBID != nil {
		params.Set("tvdbId", strconv.FormatInt(*q.TVDBID, 10))
	}
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProwlarrUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrInvalidAPIKey
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prowlarr returned status %d", resp.StatusCode)
	}

	var rawReleases []prowlarrRelease
	if err := json.NewDecoder(resp.Body).Decode(&rawReleases); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	releases := make([]ProwlarrRelease, len(rawReleases))
	for i, r := range rawReleases {
		pubDate, _ := time.Parse(time.RFC3339, r.PublishDate)
		releases[i] = ProwlarrRelease{
			Title:       r.Title,
			GUID:        r.GUID,
			Indexer:     r.Indexer,
			DownloadURL: r.DownloadURL,
			Size:        r.Size,
			PublishDate: pubDate,
		}
	}

	return releases, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/search/... -v -run TestProwlarrClient`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/search/prowlarr.go internal/search/prowlarr_test.go
git commit -m "feat(search): implement Prowlarr HTTP client"
```

---

### Task 6: Searcher Orchestration

**Files:**
- Modify: `internal/search/search.go` (replace stub)
- Create: `internal/search/search_test.go`

**Step 1: Write the test**

```go
// internal/search/search_test.go
package search

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockProwlarrAPI implements ProwlarrAPI for testing
type mockProwlarrAPI struct {
	releases []ProwlarrRelease
	err      error
}

func (m *mockProwlarrAPI) Search(ctx context.Context, q Query) ([]ProwlarrRelease, error) {
	return m.releases, m.err
}

func TestSearcher_Search(t *testing.T) {
	profiles := map[string][]string{
		"hd": {"1080p bluray", "1080p webdl", "720p bluray"},
	}
	scorer := NewScorer(profiles)

	mockClient := &mockProwlarrAPI{
		releases: []ProwlarrRelease{
			{Title: "Movie.2024.1080p.BluRay.x264-GROUP", GUID: "1", Indexer: "Test", Size: 5000000000},
			{Title: "Movie.2024.720p.BluRay.x264-OTHER", GUID: "2", Indexer: "Test", Size: 2000000000},
			{Title: "Movie.2024.480p.DVDRip.x264-BAD", GUID: "3", Indexer: "Test", Size: 700000000},
			{Title: "Movie.2024.1080p.WEB-DL.x264-WEB", GUID: "4", Indexer: "Test", Size: 4000000000},
		},
	}

	searcher := NewSearcher(mockClient, scorer)
	result, err := searcher.Search(context.Background(), Query{Text: "Movie 2024"}, "hd")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	// Should filter out 480p DVDRip (score 0)
	if len(result.Releases) != 3 {
		t.Fatalf("expected 3 releases after filtering, got %d", len(result.Releases))
	}

	// Should be sorted by score descending
	// 1080p bluray (3) > 1080p webdl (2) > 720p bluray (1)
	if result.Releases[0].GUID != "1" {
		t.Errorf("expected GUID 1 first, got %s", result.Releases[0].GUID)
	}
	if result.Releases[1].GUID != "4" {
		t.Errorf("expected GUID 4 second, got %s", result.Releases[1].GUID)
	}
	if result.Releases[2].GUID != "2" {
		t.Errorf("expected GUID 2 third, got %s", result.Releases[2].GUID)
	}

	// Verify scores are set
	if result.Releases[0].Score != 3 {
		t.Errorf("expected score 3, got %d", result.Releases[0].Score)
	}
}

func TestSearcher_Search_NoMatches(t *testing.T) {
	profiles := map[string][]string{
		"uhd": {"2160p bluray"},
	}
	scorer := NewScorer(profiles)

	mockClient := &mockProwlarrAPI{
		releases: []ProwlarrRelease{
			{Title: "Movie.2024.1080p.BluRay.x264-GROUP", GUID: "1"},
		},
	}

	searcher := NewSearcher(mockClient, scorer)
	result, err := searcher.Search(context.Background(), Query{Text: "Movie 2024"}, "uhd")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(result.Releases) != 0 {
		t.Errorf("expected 0 releases, got %d", len(result.Releases))
	}
}

func TestSearcher_Search_ClientError(t *testing.T) {
	scorer := NewScorer(nil)
	mockClient := &mockProwlarrAPI{
		err: errors.New("connection refused"),
	}

	searcher := NewSearcher(mockClient, scorer)
	result, err := searcher.Search(context.Background(), Query{Text: "Movie 2024"}, "hd")

	// Should still return a result with the error
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error in result, got %d", len(result.Errors))
	}
	if len(result.Releases) != 0 {
		t.Errorf("expected 0 releases on error, got %d", len(result.Releases))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/search/... -v -run "TestSearcher"`
Expected: FAIL (Searcher undefined or wrong signature)

**Step 3: Write minimal implementation**

Replace `internal/search/search.go`:

```go
// Package search handles indexer queries and release matching.
package search

import (
	"context"
	"sort"
	"time"

	"github.com/user/arrgo/pkg/release"
)

// Release represents a scored search result.
type Release struct {
	Title       string
	Indexer     string
	GUID        string
	DownloadURL string
	Size        int64
	PublishDate time.Time
	Quality     release.Info // Parsed quality info
	Score       int          // Match score (higher is better)
}

// Query specifies what to search for.
type Query struct {
	ContentID int64  // If searching for known content
	Text      string // Free text search
	Type      string // "movie" or "series"
	TMDBID    *int64
	TVDBID    *int64
	Season    *int
	Episode   *int
}

// SearchResult contains search results and any errors encountered.
type SearchResult struct {
	Releases []*Release
	Errors   []error
}

// ProwlarrAPI defines the interface for Prowlarr clients.
type ProwlarrAPI interface {
	Search(ctx context.Context, q Query) ([]ProwlarrRelease, error)
}

// Searcher orchestrates searching, parsing, and scoring.
type Searcher struct {
	client ProwlarrAPI
	scorer *Scorer
}

// NewSearcher creates a new searcher.
func NewSearcher(client ProwlarrAPI, scorer *Scorer) *Searcher {
	return &Searcher{
		client: client,
		scorer: scorer,
	}
}

// Search queries Prowlarr, parses results, scores against profile, and returns sorted results.
func (s *Searcher) Search(ctx context.Context, q Query, profile string) (*SearchResult, error) {
	result := &SearchResult{}

	// Query Prowlarr
	prowlarrReleases, err := s.client.Search(ctx, q)
	if err != nil {
		result.Errors = append(result.Errors, err)
		return result, nil
	}

	// Parse and score each release
	for _, pr := range prowlarrReleases {
		info := release.Parse(pr.Title)
		score := s.scorer.Score(info, profile)

		// Filter out non-matching releases
		if score == 0 {
			continue
		}

		result.Releases = append(result.Releases, &Release{
			Title:       pr.Title,
			Indexer:     pr.Indexer,
			GUID:        pr.GUID,
			DownloadURL: pr.DownloadURL,
			Size:        pr.Size,
			PublishDate: pr.PublishDate,
			Quality:     info,
			Score:       score,
		})
	}

	// Sort by score descending
	sort.Slice(result.Releases, func(i, j int) bool {
		return result.Releases[i].Score > result.Releases[j].Score
	})

	return result, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/search/... -v -run "TestSearcher"`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/search/search.go internal/search/search_test.go
git commit -m "feat(search): implement Searcher orchestration with scoring and sorting"
```

---

### Task 7: Update Module Path and Final Verification

**Files:**
- Modify: all test files to use correct module path

**Step 1: Check go.mod for module name**

Run: `head -1 go.mod`

**Step 2: Update imports if needed**

The import path should match your go.mod module name. If it's `github.com/user/arrgo`, update all `github.com/user/arrgo` imports accordingly.

**Step 3: Run all tests**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 4: Run linter**

Run: `golangci-lint run ./...`
Expected: No issues (or fix any that appear)

**Step 5: Verify build**

Run: `go build ./...`
Expected: Success

**Step 6: Final commit (if any fixes)**

```bash
git add -A
git commit -m "fix(search): resolve lint issues and verify build"
```

---

## Summary

| Task | Component | Description |
|------|-----------|-------------|
| 1 | pkg/release | Release types (Resolution, Source, Codec enums) |
| 2 | pkg/release | Parse function with regex patterns |
| 3 | internal/search | Error types |
| 4 | internal/search | Quality scorer with profile matching |
| 5 | internal/search | Prowlarr HTTP client |
| 6 | internal/search | Searcher orchestration |
| 7 | all | Final verification (tests, lint, build) |

## Test Data Note

The plan includes ~30 test cases for the release parser. After implementation, expand to 1000+ real examples by:
1. Querying public indexer APIs
2. Extracting from Prowlarr/Radarr/Sonarr logs
3. Mining *arr GitHub issues for edge cases
4. Reviewing scene naming standards

Store expanded test data in `pkg/release/testdata/releases.json` with format:
```json
[
  {"title": "...", "resolution": "1080p", "source": "bluray", "codec": "x264"},
  ...
]
```
