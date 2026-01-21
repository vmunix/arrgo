-- Add 'importing' status to downloads table.
-- SQLite doesn't support ALTER TABLE to modify CHECK constraints,
-- so we recreate the table with the updated constraint.

CREATE TABLE downloads_new (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    content_id      INTEGER NOT NULL REFERENCES content(id) ON DELETE CASCADE,
    episode_id      INTEGER REFERENCES episodes(id) ON DELETE CASCADE,
    client          TEXT NOT NULL CHECK (client IN ('sabnzbd', 'qbittorrent', 'manual')),
    client_id       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'downloading', 'completed', 'importing', 'failed', 'imported', 'cleaned')),
    release_name    TEXT,
    indexer         TEXT,
    added_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at    DATETIME,
    last_transition_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(client, client_id)
);

INSERT INTO downloads_new SELECT * FROM downloads;
DROP TABLE downloads;
ALTER TABLE downloads_new RENAME TO downloads;

CREATE INDEX idx_downloads_content ON downloads(content_id);
CREATE INDEX idx_downloads_status ON downloads(status);
