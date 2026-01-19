-- Add 'cleaned' to downloads status CHECK constraint
-- SQLite doesn't support ALTER CHECK, so we recreate the table

CREATE TABLE downloads_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    content_id      INTEGER NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    episode_id      INTEGER REFERENCES episodes(id) ON DELETE CASCADE,
    client          TEXT NOT NULL CHECK (client IN ('sabnzbd', 'qbittorrent', 'manual')),
    client_id       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'downloading', 'completed', 'failed', 'imported', 'cleaned')),
    release_name    TEXT,
    indexer         TEXT,
    added_at        TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at    TIMESTAMP,
    last_transition_at TIMESTAMP
);

INSERT INTO downloads_new SELECT * FROM downloads;
DROP TABLE downloads;
ALTER TABLE downloads_new RENAME TO downloads;

CREATE INDEX IF NOT EXISTS idx_downloads_content ON downloads(content_id);
CREATE INDEX IF NOT EXISTS idx_downloads_status ON downloads(status);
CREATE INDEX IF NOT EXISTS idx_downloads_client ON downloads(client, client_id);
