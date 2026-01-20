package search_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/internal/search"
	"github.com/vmunix/arrgo/internal/search/mocks"
	"github.com/vmunix/arrgo/pkg/release"
	"go.uber.org/mock/gomock"
)

// testLogger returns a discard logger for tests.
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSearcher_Search(t *testing.T) {
	ctrl := gomock.NewController(t)

	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p", "720p"},
			Sources:    []string{"bluray", "webdl"},
		},
	}
	scorer := search.NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	mockClient.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		Return([]search.Release{
			{Title: "Movie.2024.1080p.BluRay.x264-GROUP", GUID: "1", Indexer: "nzbgeek"},
			{Title: "Movie.2024.720p.BluRay.x264-OTHER", GUID: "2", Indexer: "nzbgeek"},
			{Title: "Movie.2024.480p.DVDRip.x264-BAD", GUID: "3", Indexer: "nzbgeek"},
			{Title: "Movie.2024.1080p.WEB-DL.x264-WEB", GUID: "4", Indexer: "nzbgeek"},
		}, nil)

	searcher := search.NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), search.Query{Text: "Movie"}, "hd")

	require.NoError(t, err)
	assert.Len(t, result.Releases, 3, "should filter out 480p DVDRip")

	// Verify sorted by score (1080p BluRay first)
	require.NotEmpty(t, result.Releases)
	assert.Equal(t, "1", result.Releases[0].GUID)
}

func TestSearcher_Search_ParsesQualityInfo(t *testing.T) {
	ctrl := gomock.NewController(t)

	profiles := map[string]config.QualityProfile{
		"any": {Resolution: []string{"1080p"}},
	}
	scorer := search.NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	mockClient.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		Return([]search.Release{
			{
				Title:       "Movie.2024.1080p.BluRay.x264-GROUP",
				GUID:        "1",
				Indexer:     "nzbgeek",
				DownloadURL: "http://example.com/download/1",
				Size:        5000000000,
				PublishDate: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC),
			},
		}, nil)

	searcher := search.NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), search.Query{Text: "Movie"}, "any")

	require.NoError(t, err)
	require.Len(t, result.Releases, 1)

	r := result.Releases[0]

	// Check that all fields are populated
	assert.Equal(t, "Movie.2024.1080p.BluRay.x264-GROUP", r.Title)
	assert.Equal(t, "nzbgeek", r.Indexer)
	assert.Equal(t, "http://example.com/download/1", r.DownloadURL)
	assert.Equal(t, int64(5000000000), r.Size)

	// Check parsed quality info
	require.NotNil(t, r.Quality, "Expected Quality to be populated")
	assert.Equal(t, release.Resolution1080p, r.Quality.Resolution)
	assert.Equal(t, release.SourceBluRay, r.Quality.Source)
	assert.Equal(t, release.CodecX264, r.Quality.Codec)
}

func TestSearcher_Search_Error(t *testing.T) {
	ctrl := gomock.NewController(t)

	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"1080p"}, Sources: []string{"bluray"}},
	}
	scorer := search.NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	mockClient.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		Return(nil, []error{errors.New("indexer unavailable")})

	searcher := search.NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), search.Query{Text: "Movie"}, "hd")

	require.NoError(t, err) // errors are collected, not returned
	assert.Empty(t, result.Releases)
	assert.Len(t, result.Errors, 1)
}

func TestSearcher_Search_NoMatches(t *testing.T) {
	ctrl := gomock.NewController(t)

	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p"},
			Sources:    []string{"bluray"},
		},
	}
	scorer := search.NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	mockClient.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		Return([]search.Release{
			{Title: "Movie.2024.480p.DVDRip.x264-BAD", GUID: "1", Indexer: "nzbgeek"},
		}, nil)

	searcher := search.NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), search.Query{Text: "Movie"}, "hd")

	require.NoError(t, err)
	assert.Empty(t, result.Releases)
}

func TestSearcher_Search_AllFiltered(t *testing.T) {
	ctrl := gomock.NewController(t)

	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"2160p"}}, // Only 4K accepted
	}
	scorer := search.NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	mockClient.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		Return([]search.Release{
			{Title: "Movie.2024.1080p.BluRay.x264-GROUP", GUID: "1"},
			{Title: "Movie.2024.720p.BluRay.x264-OTHER", GUID: "2"},
		}, nil)

	searcher := search.NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), search.Query{Text: "Movie"}, "hd")

	require.NoError(t, err)
	// All releases should be filtered out (score=0)
	assert.Empty(t, result.Releases, "Expected all releases to be filtered")
}

func TestSearcher_Search_UnknownProfile(t *testing.T) {
	ctrl := gomock.NewController(t)

	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"1080p"}, Sources: []string{"bluray"}},
	}
	scorer := search.NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	mockClient.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		Return([]search.Release{
			{Title: "Movie.2024.1080p.BluRay.x264-GROUP", GUID: "1"},
		}, nil)

	searcher := search.NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), search.Query{Text: "Movie"}, "nonexistent")

	require.NoError(t, err)
	// Unknown profile means all releases get score 0 and are filtered
	assert.Empty(t, result.Releases, "Expected all releases to be filtered for unknown profile")
}

func TestSearcher_Search_SortStability(t *testing.T) {
	ctrl := gomock.NewController(t)

	// Test that releases with the same score maintain stable ordering
	profiles := map[string]config.QualityProfile{
		"hd": {Resolution: []string{"1080p"}},
	}
	scorer := search.NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	mockClient.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		Return([]search.Release{
			{Title: "Movie.2024.1080p.BluRay.x264-AAA", GUID: "1"},
			{Title: "Movie.2024.1080p.WEB-DL.x264-BBB", GUID: "2"},
			{Title: "Movie.2024.1080p.BluRay.x265-CCC", GUID: "3"},
		}, nil)

	searcher := search.NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), search.Query{Text: "Movie"}, "hd")

	require.NoError(t, err)
	// All have score 80 (only 1080p in profile, no source preference)
	// Should maintain original order due to stable sort
	require.Len(t, result.Releases, 3)

	expectedGUIDs := []string{"1", "2", "3"}
	for i, expected := range expectedGUIDs {
		assert.Equal(t, expected, result.Releases[i].GUID, "Position %d: expected GUID %s", i, expected)
	}
}

func TestSearcher_SequelPenalty(t *testing.T) {
	ctrl := gomock.NewController(t)

	profiles := map[string]config.QualityProfile{
		"hd": {
			Resolution: []string{"1080p"},
			Sources:    []string{"webdl"},
		},
	}
	scorer := search.NewScorer(profiles)

	mockClient := mocks.NewMockIndexerAPI(ctrl)
	mockClient.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		Return([]search.Release{
			{Title: "Back.to.the.Future.Part.III.1990.1080p.WEB-DL.x264", GUID: "sequel", Indexer: "test"},
			{Title: "Back.to.the.Future.1985.1080p.WEB-DL.x264", GUID: "original", Indexer: "test"},
		}, nil)

	searcher := search.NewSearcher(mockClient, scorer, testLogger())
	result, err := searcher.Search(context.Background(), search.Query{Text: "Back to the Future"}, "hd")

	require.NoError(t, err)
	require.Len(t, result.Releases, 2)

	// Original should rank higher (first) due to sequel penalty
	assert.Equal(t, "original", result.Releases[0].GUID, "Expected original first")
	assert.Equal(t, "sequel", result.Releases[1].GUID, "Expected sequel second")
}
