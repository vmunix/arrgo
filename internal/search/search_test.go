package search

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/arrgo/arrgo/internal/config"
	"github.com/arrgo/arrgo/pkg/release"
)

// testLogger returns a discard logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockIndexerAPI implements IndexerAPI for testing.
type mockIndexerAPI struct {
	releases []Release
	errs     []error
}

func (m *mockIndexerAPI) Search(ctx context.Context, q Query) ([]Release, []error) {
	return m.releases, m.errs
}

func TestSearcher_Search(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p", "720p"},
			Sources:    []string{"bluray", "webdl"},
		},
	}
	scorer := NewScorer(profiles)

	mockClient := &mockIndexerAPI{
		releases: []Release{
			{Title: "Movie.2024.1080p.BluRay.x264-GROUP", GUID: "1", Indexer: "nzbgeek"},
			{Title: "Movie.2024.720p.BluRay.x264-OTHER", GUID: "2", Indexer: "nzbgeek"},
			{Title: "Movie.2024.480p.DVDRip.x264-BAD", GUID: "3", Indexer: "nzbgeek"},   // should be filtered
			{Title: "Movie.2024.1080p.WEB-DL.x264-WEB", GUID: "4", Indexer: "nzbgeek"},
		},
	}

	searcher := NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), Query{Text: "Movie"}, "hd")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	// Should have no errors
	if len(result.Errors) > 0 {
		t.Errorf("Expected no errors, got %v", result.Errors)
	}

	// Should filter out 480p (score=0), leaving 3 releases
	if len(result.Releases) != 3 {
		t.Fatalf("Expected 3 releases after filtering, got %d", len(result.Releases))
	}

	// Expected order by score:
	// 1080p bluray: base 80 + source 10 = 90
	// 1080p webdl: base 80 + source 8 = 88
	// 720p bluray: base 60 + source 10 = 70
	expectedOrder := []struct {
		guid  string
		score int
	}{
		{"1", 90}, // 1080p bluray
		{"4", 88}, // 1080p webdl
		{"2", 70}, // 720p bluray
	}

	for i, expected := range expectedOrder {
		if result.Releases[i].GUID != expected.guid {
			t.Errorf("Position %d: expected GUID %s, got %s", i, expected.guid, result.Releases[i].GUID)
		}
		if result.Releases[i].Score != expected.score {
			t.Errorf("Position %d (GUID %s): expected score %d, got %d",
				i, result.Releases[i].GUID, expected.score, result.Releases[i].Score)
		}
	}
}

func TestSearcher_Search_ParsesQualityInfo(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"any": {Resolution: []string{"1080p"}},
	}
	scorer := NewScorer(profiles)

	mockClient := &mockIndexerAPI{
		releases: []Release{
			{
				Title:       "Movie.2024.1080p.BluRay.x264-GROUP",
				GUID:        "1",
				Indexer:     "nzbgeek",
				DownloadURL: "http://example.com/download/1",
				Size:        5000000000,
				PublishDate: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	searcher := NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), Query{Text: "Movie"}, "any")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(result.Releases) != 1 {
		t.Fatalf("Expected 1 release, got %d", len(result.Releases))
	}

	r := result.Releases[0]

	// Check that all fields are populated
	if r.Title != "Movie.2024.1080p.BluRay.x264-GROUP" {
		t.Errorf("Expected title 'Movie.2024.1080p.BluRay.x264-GROUP', got %s", r.Title)
	}
	if r.Indexer != "nzbgeek" {
		t.Errorf("Expected indexer 'nzbgeek', got %s", r.Indexer)
	}
	if r.DownloadURL != "http://example.com/download/1" {
		t.Errorf("Expected download URL 'http://example.com/download/1', got %s", r.DownloadURL)
	}
	if r.Size != 5000000000 {
		t.Errorf("Expected size 5000000000, got %d", r.Size)
	}

	// Check parsed quality info
	if r.Quality == nil {
		t.Fatal("Expected Quality to be populated")
	}
	if r.Quality.Resolution != release.Resolution1080p {
		t.Errorf("Expected resolution 1080p, got %s", r.Quality.Resolution)
	}
	if r.Quality.Source != release.SourceBluRay {
		t.Errorf("Expected source bluray, got %s", r.Quality.Source)
	}
	if r.Quality.Codec != release.CodecX264 {
		t.Errorf("Expected codec x264, got %s", r.Quality.Codec)
	}
}

func TestSearcher_Search_ClientError(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"1080p"}},
	}
	scorer := NewScorer(profiles)

	expectedErr := errors.New("connection refused")
	mockClient := &mockIndexerAPI{
		releases: nil,
		errs:     []error{expectedErr},
	}

	searcher := NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), Query{Text: "Movie"}, "hd")

	// Should not return error (partial results pattern)
	if err != nil {
		t.Fatalf("Search should not return error directly, got: %v", err)
	}

	// Error should be added to result.Errors
	if len(result.Errors) != 1 {
		t.Fatalf("Expected 1 error in result.Errors, got %d", len(result.Errors))
	}

	if result.Errors[0] != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, result.Errors[0])
	}

	// Releases should be empty
	if len(result.Releases) != 0 {
		t.Errorf("Expected 0 releases, got %d", len(result.Releases))
	}
}

func TestSearcher_Search_EmptyResults(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"1080p"}},
	}
	scorer := NewScorer(profiles)

	mockClient := &mockIndexerAPI{
		releases: []Release{},
		errs:     nil,
	}

	searcher := NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), Query{Text: "NonExistent"}, "hd")

	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %v", result.Errors)
	}

	if len(result.Releases) != 0 {
		t.Errorf("Expected 0 releases, got %d", len(result.Releases))
	}
}

func TestSearcher_Search_AllFiltered(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"2160p"}}, // Only 4K accepted
	}
	scorer := NewScorer(profiles)

	mockClient := &mockIndexerAPI{
		releases: []Release{
			{Title: "Movie.2024.1080p.BluRay.x264-GROUP", GUID: "1"},
			{Title: "Movie.2024.720p.BluRay.x264-OTHER", GUID: "2"},
		},
	}

	searcher := NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), Query{Text: "Movie"}, "hd")

	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	// All releases should be filtered out (score=0)
	if len(result.Releases) != 0 {
		t.Errorf("Expected all releases to be filtered, got %d", len(result.Releases))
	}
}

func TestSearcher_Search_UnknownProfile(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"1080p"}},
	}
	scorer := NewScorer(profiles)

	mockClient := &mockIndexerAPI{
		releases: []Release{
			{Title: "Movie.2024.1080p.BluRay.x264-GROUP", GUID: "1"},
		},
	}

	searcher := NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), Query{Text: "Movie"}, "nonexistent")

	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	// Unknown profile means all releases get score 0 and are filtered
	if len(result.Releases) != 0 {
		t.Errorf("Expected all releases to be filtered for unknown profile, got %d", len(result.Releases))
	}
}

func TestSearcher_Search_SortStability(t *testing.T) {
	// Test that releases with the same score maintain stable ordering
	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"1080p"}},
	}
	scorer := NewScorer(profiles)

	mockClient := &mockIndexerAPI{
		releases: []Release{
			{Title: "Movie.2024.1080p.BluRay.x264-AAA", GUID: "1"},
			{Title: "Movie.2024.1080p.WEB-DL.x264-BBB", GUID: "2"},
			{Title: "Movie.2024.1080p.BluRay.x265-CCC", GUID: "3"},
		},
	}

	searcher := NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), Query{Text: "Movie"}, "hd")

	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	// All have score 80 (only 1080p in profile, no source preference)
	// Should maintain original order due to stable sort
	if len(result.Releases) != 3 {
		t.Fatalf("Expected 3 releases, got %d", len(result.Releases))
	}

	expectedGUIDs := []string{"1", "2", "3"}
	for i, expected := range expectedGUIDs {
		if result.Releases[i].GUID != expected {
			t.Errorf("Position %d: expected GUID %s, got %s", i, expected, result.Releases[i].GUID)
		}
	}
}

func TestHasSequelMismatch(t *testing.T) {
	tests := []struct {
		query    string
		title    string
		mismatch bool
	}{
		// No sequel in either - no mismatch
		{"Back to the Future", "Back to the Future", false},
		// Sequel in title but not query - mismatch
		{"Back to the Future", "Back to the Future Part II", true},
		{"Back to the Future", "Back to the Future Part III", true},
		{"Back to the Future", "Back.to.the.Future.II.1989", true},   // Roman numeral only
		{"The Matrix", "The.Matrix.III.2003", true},                  // Roman numeral only
		// Sequel in both - matching numbers - no mismatch
		{"Back to the Future Part II", "Back to the Future Part II", false},
		{"Back to the Future Part 2", "Back to the Future Part II", false}, // Part 2 matches II
		{"Back to the Future Part 2", "Back.to.the.Future.II.1989", false}, // Part 2 matches II
		// Sequel in both - different numbers - mismatch
		{"Back to the Future Part 2", "Back to the Future Part III", true}, // Part 2 != III
		{"Back to the Future Part II", "Back.to.the.Future.III.1990", true}, // II != III
		// Query has sequel, title doesn't - no mismatch (original is fine)
		{"Back to the Future Part II", "Back to the Future", false},
		// Audio specs should NOT trigger sequel detection
		{"Back to the Future", "Back.to.the.Future.1985.DD.5.1", false},
	}

	for _, tc := range tests {
		got := hasSequelMismatch(tc.query, tc.title)
		if got != tc.mismatch {
			t.Errorf("hasSequelMismatch(%q, %q) = %v, want %v", tc.query, tc.title, got, tc.mismatch)
		}
	}
}

func TestSearcher_SequelPenalty(t *testing.T) {
	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p"},
			Sources:    []string{"webdl"},
		},
	}
	scorer := NewScorer(profiles)

	mockClient := &mockIndexerAPI{
		releases: []Release{
			{Title: "Back.to.the.Future.Part.III.1990.1080p.WEB-DL.x264", GUID: "sequel", Indexer: "test"},
			{Title: "Back.to.the.Future.1985.1080p.WEB-DL.x264", GUID: "original", Indexer: "test"},
		},
	}

	searcher := NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), Query{Text: "Back to the Future"}, "hd")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(result.Releases) != 2 {
		t.Fatalf("Expected 2 releases, got %d", len(result.Releases))
	}

	// Original should rank higher (first) due to sequel penalty
	if result.Releases[0].GUID != "original" {
		t.Errorf("Expected original first, got %s", result.Releases[0].GUID)
	}
	if result.Releases[1].GUID != "sequel" {
		t.Errorf("Expected sequel second, got %s", result.Releases[1].GUID)
	}
}
