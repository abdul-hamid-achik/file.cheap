-- Add video processing seconds tracking to monthly_usage table
ALTER TABLE monthly_usage ADD COLUMN IF NOT EXISTS video_seconds_processed INTEGER NOT NULL DEFAULT 0;

-- Add index for efficient cleanup queries
CREATE INDEX IF NOT EXISTS idx_files_deleted_at ON files(deleted_at) WHERE deleted_at IS NOT NULL;

-- Add index for retention cleanup (files older than retention period)
CREATE INDEX IF NOT EXISTS idx_files_created_at_for_cleanup ON files(created_at, user_id) WHERE deleted_at IS NULL;
