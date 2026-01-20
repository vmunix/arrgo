package tmdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_GetSet(t *testing.T) {
	c := newCache(time.Hour)

	// Miss
	_, ok := c.get(12345)
	assert.False(t, ok, "empty cache should miss")

	// Set and hit
	movie := &Movie{ID: 12345, Title: "Test Movie"}
	c.set(12345, movie)

	got, ok := c.get(12345)
	require.True(t, ok, "should hit after set")
	assert.Equal(t, "Test Movie", got.Title)

	// Different ID should miss
	_, ok = c.get(99999)
	assert.False(t, ok, "different ID should miss")

	// Set another movie
	movie2 := &Movie{ID: 99999, Title: "Another Movie"}
	c.set(99999, movie2)

	got2, ok := c.get(99999)
	require.True(t, ok, "should hit second movie")
	assert.Equal(t, "Another Movie", got2.Title)

	// First movie should still be there
	got, ok = c.get(12345)
	require.True(t, ok, "first movie should still exist")
	assert.Equal(t, "Test Movie", got.Title)
}

func TestCache_Expiry(t *testing.T) {
	c := newCache(10 * time.Millisecond)

	c.set(12345, &Movie{ID: 12345, Title: "Test"})

	// Should hit immediately
	_, ok := c.get(12345)
	require.True(t, ok)

	// Wait for expiry
	time.Sleep(20 * time.Millisecond)

	// Should miss after expiry
	_, ok = c.get(12345)
	assert.False(t, ok, "should miss after TTL")
}
