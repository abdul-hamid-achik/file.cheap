-- Migration: Add video support with variant types and metadata columns
-- This migration adds video-specific variant types and extends file_variants with video metadata

BEGIN;

-- Add video variant types to the enum
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'video_thumbnail';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'video_sprite';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'mp4_360p';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'mp4_480p';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'mp4_720p';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'mp4_1080p';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'mp4_2160p';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'webm_720p';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'webm_1080p';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'hls_master';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'hls_360p';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'hls_480p';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'hls_720p';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'hls_1080p';
ALTER TYPE variant_type ADD VALUE IF NOT EXISTS 'video_watermarked';

-- Add video job types
ALTER TYPE job_type ADD VALUE IF NOT EXISTS 'video_thumbnail';
ALTER TYPE job_type ADD VALUE IF NOT EXISTS 'video_transcode';
ALTER TYPE job_type ADD VALUE IF NOT EXISTS 'video_hls';
ALTER TYPE job_type ADD VALUE IF NOT EXISTS 'video_watermark';

-- Add video metadata columns to file_variants
ALTER TABLE file_variants ADD COLUMN IF NOT EXISTS duration_seconds NUMERIC(10, 2);
ALTER TABLE file_variants ADD COLUMN IF NOT EXISTS bitrate_bps BIGINT;
ALTER TABLE file_variants ADD COLUMN IF NOT EXISTS video_codec VARCHAR(50);
ALTER TABLE file_variants ADD COLUMN IF NOT EXISTS audio_codec VARCHAR(50);
ALTER TABLE file_variants ADD COLUMN IF NOT EXISTS frame_rate NUMERIC(6, 2);
ALTER TABLE file_variants ADD COLUMN IF NOT EXISTS resolution VARCHAR(20);

-- Add video usage tracking to users
ALTER TABLE users ADD COLUMN IF NOT EXISTS video_minutes_used INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS video_minutes_reset_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS video_storage_bytes_used BIGINT NOT NULL DEFAULT 0;

-- Create index for video variants (for listing video outputs)
CREATE INDEX IF NOT EXISTS idx_file_variants_video ON file_variants(file_id)
    WHERE variant_type IN ('mp4_360p', 'mp4_480p', 'mp4_720p', 'mp4_1080p', 'mp4_2160p',
                           'webm_720p', 'webm_1080p', 'hls_master');

-- Add comments for documentation
COMMENT ON COLUMN file_variants.duration_seconds IS 'Video duration in seconds';
COMMENT ON COLUMN file_variants.bitrate_bps IS 'Video bitrate in bits per second';
COMMENT ON COLUMN file_variants.video_codec IS 'Video codec (e.g., h264, vp9, hevc)';
COMMENT ON COLUMN file_variants.audio_codec IS 'Audio codec (e.g., aac, opus, mp3)';
COMMENT ON COLUMN file_variants.frame_rate IS 'Video frame rate (fps)';
COMMENT ON COLUMN file_variants.resolution IS 'Video resolution (e.g., 1920x1080)';
COMMENT ON COLUMN users.video_minutes_used IS 'Video processing minutes used this billing period';
COMMENT ON COLUMN users.video_minutes_reset_at IS 'When video minutes quota resets';
COMMENT ON COLUMN users.video_storage_bytes_used IS 'Total video storage used in bytes';

COMMIT;
