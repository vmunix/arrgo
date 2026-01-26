-- Metadata cache for external API responses (TVDB, TMDB)
CREATE TABLE IF NOT EXISTS metadata_cache (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    expires_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_metadata_cache_expires ON metadata_cache(expires_at);

INSERT OR IGNORE INTO schema_migrations (version) VALUES (7);
