-- Test schema for library module
PRAGMA foreign_keys = ON;

CREATE TABLE content (
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

CREATE INDEX idx_content_type ON content(type);
CREATE INDEX idx_content_status ON content(status);
CREATE INDEX idx_content_tmdb ON content(tmdb_id);
CREATE INDEX idx_content_tvdb ON content(tvdb_id);

CREATE TABLE episodes (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    content_id      INTEGER NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    season          INTEGER NOT NULL,
    episode         INTEGER NOT NULL,
    title           TEXT,
    status          TEXT NOT NULL DEFAULT 'wanted' CHECK (status IN ('wanted', 'available', 'unmonitored')),
    air_date        DATE,
    UNIQUE(content_id, season, episode)
);

CREATE INDEX idx_episodes_content ON episodes(content_id);
CREATE INDEX idx_episodes_status ON episodes(status);

CREATE TABLE files (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    content_id      INTEGER NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    episode_id      INTEGER REFERENCES episodes(id) ON DELETE CASCADE,
    path            TEXT NOT NULL UNIQUE,
    size_bytes      INTEGER,
    quality         TEXT,
    source          TEXT,
    added_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_files_content ON files(content_id);
CREATE INDEX idx_files_episode ON files(episode_id);
