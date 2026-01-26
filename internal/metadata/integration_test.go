//go:build integration

package metadata

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmunix/arrgo/pkg/tvdb"
)

func TestTVDB_Integration(t *testing.T) {
	apiKey := os.Getenv("TVDB_API_KEY")
	if apiKey == "" {
		t.Skip("TVDB_API_KEY not set")
	}

	client := tvdb.New(apiKey)
	ctx := context.Background()

	// Test search
	results, err := client.Search(ctx, "Breaking Bad")
	require.NoError(t, err)
	require.NotEmpty(t, results)

	// Find Breaking Bad
	var bbID int
	for _, r := range results {
		if r.Name == "Breaking Bad" {
			bbID = r.ID
			break
		}
	}
	require.NotZero(t, bbID, "Breaking Bad not found in search results")

	// Test get series
	series, err := client.GetSeries(ctx, bbID)
	require.NoError(t, err)
	require.Equal(t, "Breaking Bad", series.Name)
	require.Equal(t, 2008, series.Year)

	// Test get episodes
	episodes, err := client.GetEpisodes(ctx, bbID)
	require.NoError(t, err)
	require.NotEmpty(t, episodes)
	t.Logf("Found %d episodes for Breaking Bad", len(episodes))
}
