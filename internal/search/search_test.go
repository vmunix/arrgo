package search

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/arrgo/arrgo/pkg/release"
)

// testLogger returns a discard logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
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
	profiles := map[string][]string{
		"hd": {"1080p bluray", "1080p webdl", "720p bluray"},
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

	// Expected order by score: 1080p bluray (3), 1080p webdl (2), 720p bluray (1)
	expectedOrder := []struct {
		guid  string
		score int
	}{
		{"1", 3}, // 1080p bluray
		{"4", 2}, // 1080p webdl
		{"2", 1}, // 720p bluray
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
	profiles := map[string][]string{
		"any": {"1080p"},
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
	profiles := map[string][]string{
		"hd": {"1080p"},
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
	profiles := map[string][]string{
		"hd": {"1080p"},
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
	profiles := map[string][]string{
		"hd": {"2160p"}, // Only 4K accepted
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
	profiles := map[string][]string{
		"hd": {"1080p"},
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
	profiles := map[string][]string{
		"hd": {"1080p"},
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

	// All have score 1 (only 1080p in profile, any source)
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
