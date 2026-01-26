-- Add progress tracking columns to downloads table
-- These are updated by the SABnzbd adapter on each poll

ALTER TABLE downloads ADD COLUMN progress REAL DEFAULT 0;
ALTER TABLE downloads ADD COLUMN speed INTEGER DEFAULT 0;
ALTER TABLE downloads ADD COLUMN eta_seconds INTEGER DEFAULT 0;
ALTER TABLE downloads ADD COLUMN size_bytes INTEGER DEFAULT 0;
