package tmdb

import (
	"sync"
	"time"
)

type cacheEntry struct {
	movie   *Movie
	expires time.Time
}

type cache struct {
	mu      sync.RWMutex
	entries map[int64]cacheEntry
	ttl     time.Duration
}

func newCache(ttl time.Duration) *cache {
	return &cache{
		entries: make(map[int64]cacheEntry),
		ttl:     ttl,
	}
}

func (c *cache) get(tmdbID int64) (*Movie, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[tmdbID]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.movie, true
}

func (c *cache) set(tmdbID int64, movie *Movie) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[tmdbID] = cacheEntry{
		movie:   movie,
		expires: time.Now().Add(c.ttl),
	}
}
