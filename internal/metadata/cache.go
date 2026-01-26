// Package metadata provides caching and orchestration for external metadata APIs.
package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Cache provides SQLite-backed caching for metadata API responses.
type Cache struct {
	db *sql.DB
}

// NewCache creates a new metadata cache.
func NewCache(db *sql.DB) *Cache {
	return &Cache{db: db}
}

// Get retrieves a cached value by key.
// Returns nil, false if not found or expired.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	var value string
	var expiresAt time.Time

	err := c.db.QueryRowContext(ctx,
		"SELECT value, expires_at FROM metadata_cache WHERE key = ?", key,
	).Scan(&value, &expiresAt)

	if err != nil || time.Now().After(expiresAt) {
		return nil, false
	}

	return []byte(value), true
}

// Set stores a value with the given TTL.
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	expiresAt := time.Now().Add(ttl)

	_, err := c.db.ExecContext(ctx,
		`INSERT INTO metadata_cache (key, value, expires_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, expires_at = excluded.expires_at`,
		key, string(value), expiresAt,
	)
	if err != nil {
		return fmt.Errorf("cache set: %w", err)
	}
	return nil
}

// Delete removes a cached value.
func (c *Cache) Delete(ctx context.Context, key string) error {
	_, err := c.db.ExecContext(ctx, "DELETE FROM metadata_cache WHERE key = ?", key)
	if err != nil {
		return fmt.Errorf("cache delete: %w", err)
	}
	return nil
}

// Prune removes all expired entries.
// Returns the number of entries removed.
func (c *Cache) Prune(ctx context.Context) (int64, error) {
	result, err := c.db.ExecContext(ctx,
		"DELETE FROM metadata_cache WHERE expires_at < ?", time.Now(),
	)
	if err != nil {
		return 0, fmt.Errorf("cache prune: %w", err)
	}
	return result.RowsAffected()
}
