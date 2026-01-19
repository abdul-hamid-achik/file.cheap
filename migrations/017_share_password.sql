-- +goose Up
-- Add password protection and download limits to file shares

ALTER TABLE file_shares ADD COLUMN password_hash VARCHAR(255);
ALTER TABLE file_shares ADD COLUMN max_downloads INTEGER;
ALTER TABLE file_shares ADD COLUMN download_count INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_file_shares_password ON file_shares(token) WHERE password_hash IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_file_shares_password;
ALTER TABLE file_shares DROP COLUMN IF EXISTS download_count;
ALTER TABLE file_shares DROP COLUMN IF EXISTS max_downloads;
ALTER TABLE file_shares DROP COLUMN IF EXISTS password_hash;
