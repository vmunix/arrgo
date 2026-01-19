-- Initial schema for arrgo
-- Run with: arrgo migrate

-- Content: movies and series unified
CREATE TABLE IF NOT EXISTS content (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    type            TEXT NOT NULL CHECK (type IN ('movie', 'series')),
    tmdb_id         INTEGER,
    tvdb_id         INTEGER,
    title           TEXT NOT NULL,
    year            INTEGER,
    status          TEXT NOT NULL DEFAULT 'wanted' CHECK (status IN ('wanted', 'available', 'unmonitored')),
    quality_profile TEXT NOT NULL DEFAULT 'hd',
    root_path       TEXT NOT NULL,
    added_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_content_type ON content(type);
CREATE INDEX IF NOT EXISTS idx_content_status ON content(status);
CREATE INDEX IF NOT EXISTS idx_content_tmdb ON content(tmdb_id);
CREATE INDEX IF NOT EXISTS idx_content_tvdb ON content(tvdb_id);

-- Episodes: only for series
CREATE TABLE IF NOT EXISTS episodes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    content_id      INTEGER NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    season          INTEGER NOT NULL,
    episode         INTEGER NOT NULL,
    title           TEXT,
    status          TEXT NOT NULL DEFAULT 'wanted' CHECK (status IN ('wanted', 'available', 'unmonitored')),
    air_date        DATE,
    UNIQUE(content_id, season, episode)
);

CREATE INDEX IF NOT EXISTS idx_episodes_content ON episodes(content_id);
CREATE INDEX IF NOT EXISTS idx_episodes_status ON episodes(status);

-- Files: what's on disk
CREATE TABLE IF NOT EXISTS files (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    content_id      INTEGER NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    episode_id      INTEGER REFERENCES episodes(id) ON DELETE CASCADE,
    path            TEXT NOT NULL UNIQUE,
    size_bytes      INTEGER,
    quality         TEXT,
    source          TEXT,
    added_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_files_content ON files(content_id);
CREATE INDEX IF NOT EXISTS idx_files_episode ON files(episode_id);

-- Downloads: active and recent
CREATE TABLE IF NOT EXISTS downloads (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    content_id      INTEGER NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    episode_id      INTEGER REFERENCES episodes(id) ON DELETE CASCADE,
    client          TEXT NOT NULL CHECK (client IN ('sabnzbd', 'qbittorrent', 'manual')),
    client_id       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'downloading', 'completed', 'failed', 'imported')),
    release_name    TEXT,
    indexer         TEXT,
    added_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at    TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_downloads_content ON downloads(content_id);
CREATE INDEX IF NOT EXISTS idx_downloads_status ON downloads(status);
CREATE INDEX IF NOT EXISTS idx_downloads_client ON downloads(client, client_id);

-- History: audit trail
CREATE TABLE IF NOT EXISTS history (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    content_id      INTEGER NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    episode_id      INTEGER REFERENCES episodes(id) ON DELETE CASCADE,
    event           TEXT NOT NULL CHECK (event IN ('grabbed', 'imported', 'deleted', 'upgraded', 'failed')),
    data            TEXT,  -- JSON blob for event-specific details
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_history_content ON history(content_id);
CREATE INDEX IF NOT EXISTS idx_history_event ON history(event);
CREATE INDEX IF NOT EXISTS idx_history_created ON history(created_at);

-- Quality profiles
CREATE TABLE IF NOT EXISTS quality_profiles (
    name            TEXT PRIMARY KEY,
    definition      TEXT NOT NULL  -- JSON: ordered list of acceptable qualities
);

-- Default quality profiles
INSERT OR IGNORE INTO quality_profiles (name, definition) VALUES
    ('hd', '["1080p bluray", "1080p webdl", "1080p hdtv", "720p bluray", "720p webdl"]'),
    ('uhd', '["2160p bluray", "2160p webdl", "1080p bluray", "1080p webdl"]'),
    ('any', '["2160p", "1080p", "720p", "480p"]');

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version         INTEGER PRIMARY KEY,
    applied_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO schema_migrations (version) VALUES (1);
