package metadata

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

// setupTestDB creates an in-memory SQLite database with the cache schema.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)

	// Create the metadata_cache table
	_, err = db.Exec(`
		CREATE TABLE metadata_cache (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Create index on expires_at for efficient pruning
	_, err = db.Exec(`CREATE INDEX idx_metadata_cache_expires_at ON metadata_cache(expires_at)`)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func TestCache_GetSet_RoundTrip(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	key := "test-key"
	value := []byte(`{"id": 123, "name": "Test Show"}`)
	ttl := 1 * time.Hour

	// Set the value
	err := cache.Set(ctx, key, value, ttl)
	require.NoError(t, err)

	// Get the value back
	got, ok := cache.Get(ctx, key)
	assert.True(t, ok, "expected to find cached value")
	assert.Equal(t, value, got)
}

func TestCache_Get_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	// Try to get a non-existent key
	got, ok := cache.Get(ctx, "nonexistent-key")
	assert.False(t, ok, "expected not to find cached value")
	assert.Nil(t, got)
}

func TestCache_Get_Expired(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	key := "expiring-key"
	value := []byte("expiring value")
	ttl := 50 * time.Millisecond

	// Set with short TTL
	err := cache.Set(ctx, key, value, ttl)
	require.NoError(t, err)

	// Verify it's there initially
	got, ok := cache.Get(ctx, key)
	assert.True(t, ok, "expected to find cached value before expiration")
	assert.Equal(t, value, got)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Now it should be expired
	got, ok = cache.Get(ctx, key)
	assert.False(t, ok, "expected not to find cached value after expiration")
	assert.Nil(t, got)
}

func TestCache_Set_Overwrite(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	key := "overwrite-key"
	value1 := []byte("first value")
	value2 := []byte("second value")
	ttl := 1 * time.Hour

	// Set first value
	err := cache.Set(ctx, key, value1, ttl)
	require.NoError(t, err)

	// Verify first value
	got, ok := cache.Get(ctx, key)
	assert.True(t, ok)
	assert.Equal(t, value1, got)

	// Set second value (overwrite)
	err = cache.Set(ctx, key, value2, ttl)
	require.NoError(t, err)

	// Verify second value replaced first
	got, ok = cache.Get(ctx, key)
	assert.True(t, ok)
	assert.Equal(t, value2, got)
}

func TestCache_Set_OverwriteExtendsTTL(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	key := "ttl-extend-key"
	value := []byte("value")

	// Set with short TTL
	err := cache.Set(ctx, key, value, 50*time.Millisecond)
	require.NoError(t, err)

	// Wait a bit but not long enough to expire
	time.Sleep(30 * time.Millisecond)

	// Overwrite with longer TTL
	err = cache.Set(ctx, key, value, 1*time.Hour)
	require.NoError(t, err)

	// Wait past original expiration
	time.Sleep(50 * time.Millisecond)

	// Should still be valid because TTL was extended
	got, ok := cache.Get(ctx, key)
	assert.True(t, ok, "expected value to still be cached after TTL extension")
	assert.Equal(t, value, got)
}

func TestCache_Delete(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	key := "delete-key"
	value := []byte("to be deleted")
	ttl := 1 * time.Hour

	// Set the value
	err := cache.Set(ctx, key, value, ttl)
	require.NoError(t, err)

	// Verify it's there
	got, ok := cache.Get(ctx, key)
	assert.True(t, ok)
	assert.Equal(t, value, got)

	// Delete it
	err = cache.Delete(ctx, key)
	require.NoError(t, err)

	// Verify it's gone
	got, ok = cache.Get(ctx, key)
	assert.False(t, ok, "expected value to be deleted")
	assert.Nil(t, got)
}

func TestCache_Delete_NonExistent(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	// Delete a non-existent key should not error
	err := cache.Delete(ctx, "nonexistent-key")
	assert.NoError(t, err)
}

func TestCache_Prune(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	// Insert some entries with different TTLs
	err := cache.Set(ctx, "short-ttl-1", []byte("value1"), 50*time.Millisecond)
	require.NoError(t, err)
	err = cache.Set(ctx, "short-ttl-2", []byte("value2"), 50*time.Millisecond)
	require.NoError(t, err)
	err = cache.Set(ctx, "long-ttl", []byte("value3"), 1*time.Hour)
	require.NoError(t, err)

	// Wait for short TTL entries to expire
	time.Sleep(100 * time.Millisecond)

	// Prune expired entries
	pruned, err := cache.Prune(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), pruned, "expected 2 expired entries to be pruned")

	// Verify short TTL entries are gone
	_, ok := cache.Get(ctx, "short-ttl-1")
	assert.False(t, ok)
	_, ok = cache.Get(ctx, "short-ttl-2")
	assert.False(t, ok)

	// Verify long TTL entry still exists
	got, ok := cache.Get(ctx, "long-ttl")
	assert.True(t, ok)
	assert.Equal(t, []byte("value3"), got)
}

func TestCache_Prune_NoExpired(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	// Insert entries with long TTL
	err := cache.Set(ctx, "key1", []byte("value1"), 1*time.Hour)
	require.NoError(t, err)
	err = cache.Set(ctx, "key2", []byte("value2"), 1*time.Hour)
	require.NoError(t, err)

	// Prune should remove nothing
	pruned, err := cache.Prune(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), pruned, "expected no entries to be pruned")

	// Verify both entries still exist
	_, ok := cache.Get(ctx, "key1")
	assert.True(t, ok)
	_, ok = cache.Get(ctx, "key2")
	assert.True(t, ok)
}

func TestCache_Prune_EmptyCache(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	// Prune empty cache should not error
	pruned, err := cache.Prune(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), pruned)
}

func TestCache_BinaryData(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	key := "binary-key"
	// Include various bytes including null bytes and high values
	value := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0x00, 0x80}
	ttl := 1 * time.Hour

	err := cache.Set(ctx, key, value, ttl)
	require.NoError(t, err)

	got, ok := cache.Get(ctx, key)
	assert.True(t, ok)
	assert.Equal(t, value, got)
}

func TestCache_EmptyValue(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	key := "empty-value-key"
	value := []byte{}
	ttl := 1 * time.Hour

	err := cache.Set(ctx, key, value, ttl)
	require.NoError(t, err)

	got, ok := cache.Get(ctx, key)
	assert.True(t, ok)
	assert.Equal(t, value, got)
}

func TestCache_LargeValue(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	key := "large-key"
	// Create a 1MB value
	value := make([]byte, 1024*1024)
	for i := range value {
		value[i] = byte(i % 256)
	}
	ttl := 1 * time.Hour

	err := cache.Set(ctx, key, value, ttl)
	require.NoError(t, err)

	got, ok := cache.Get(ctx, key)
	assert.True(t, ok)
	assert.Equal(t, value, got)
}

func TestCache_SpecialCharactersInKey(t *testing.T) {
	db := setupTestDB(t)
	cache := NewCache(db)
	ctx := context.Background()

	testCases := []struct {
		name string
		key  string
	}{
		{"spaces", "key with spaces"},
		{"unicode", "key-\u4e2d\u6587-\u65e5\u672c\u8a9e"},
		{"special chars", "key:with/special?chars&more=stuff"},
		{"quotes", `key"with'quotes`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			value := []byte("value for " + tc.key)
			ttl := 1 * time.Hour

			err := cache.Set(ctx, tc.key, value, ttl)
			require.NoError(t, err)

			got, ok := cache.Get(ctx, tc.key)
			assert.True(t, ok)
			assert.Equal(t, value, got)
		})
	}
}
