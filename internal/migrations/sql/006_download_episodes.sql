-- Migration 006: Add download_episodes junction table for multi-episode downloads.
-- This supports season packs and multi-episode releases.

-- Junction table for download-to-episode relationships (many-to-many)
CREATE TABLE IF NOT EXISTS download_episodes (
    download_id INTEGER NOT NULL REFERENCES downloads(id) ON DELETE CASCADE,
    episode_id  INTEGER NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    PRIMARY KEY (download_id, episode_id)
);

-- Index for efficient lookups by episode
CREATE INDEX IF NOT EXISTS idx_download_episodes_episode_id ON download_episodes(episode_id);

-- Migrate existing episode_id data from downloads table
INSERT OR IGNORE INTO download_episodes (download_id, episode_id)
SELECT id, episode_id FROM downloads WHERE episode_id IS NOT NULL;

-- Add season tracking columns to downloads
-- season: The season number this download is for
-- is_complete_season: 1 if this is a complete season pack, 0 otherwise
ALTER TABLE downloads ADD COLUMN season INTEGER;
ALTER TABLE downloads ADD COLUMN is_complete_season INTEGER DEFAULT 0;
