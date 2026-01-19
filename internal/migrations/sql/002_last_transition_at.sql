-- Add last_transition_at column for stuck detection
ALTER TABLE downloads ADD COLUMN last_transition_at TIMESTAMP;
UPDATE downloads SET last_transition_at = added_at WHERE last_transition_at IS NULL;
