-- +goose Up
-- Add storage quota columns to users table

ALTER TABLE users ADD COLUMN storage_limit_bytes BIGINT NOT NULL DEFAULT 1073741824; -- 1GB default
ALTER TABLE users ADD COLUMN storage_used_bytes BIGINT NOT NULL DEFAULT 0;

-- Update existing users based on their subscription tier
UPDATE users SET storage_limit_bytes = 1073741824 WHERE subscription_tier = 'free';      -- 1GB
UPDATE users SET storage_limit_bytes = 107374182400 WHERE subscription_tier = 'pro';     -- 100GB
UPDATE users SET storage_limit_bytes = 1099511627776 WHERE subscription_tier = 'enterprise'; -- 1TB

-- Create index for storage usage queries
CREATE INDEX idx_users_storage_usage ON users(storage_used_bytes) WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_users_storage_usage;
ALTER TABLE users DROP COLUMN IF EXISTS storage_used_bytes;
ALTER TABLE users DROP COLUMN IF EXISTS storage_limit_bytes;
