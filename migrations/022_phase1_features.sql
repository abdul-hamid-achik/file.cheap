-- Phase 1 Features Migration
-- Adds: File Tags, ZIP Downloads, Webhook Dead Letter Queue, zip_download job type

-- Add zip_download to job_type enum if it doesn't exist
DO $$ BEGIN
    ALTER TYPE job_type ADD VALUE IF NOT EXISTS 'zip_download';
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- ============================================================================
-- FILE TAGS
-- ============================================================================

-- File tags table for organizing files with labels
CREATE TABLE IF NOT EXISTS file_tags (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tag_name VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(file_id, tag_name)
);

CREATE INDEX IF NOT EXISTS idx_file_tags_file_id ON file_tags(file_id);
CREATE INDEX IF NOT EXISTS idx_file_tags_user_id ON file_tags(user_id);
CREATE INDEX IF NOT EXISTS idx_file_tags_tag_name ON file_tags(user_id, tag_name);

-- ============================================================================
-- ZIP DOWNLOADS
-- ============================================================================

-- ZIP download requests table
CREATE TABLE IF NOT EXISTS zip_downloads (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_ids UUID[] NOT NULL,
    status job_status NOT NULL DEFAULT 'pending',
    storage_key TEXT,
    size_bytes BIGINT,
    download_url TEXT,
    expires_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_zip_downloads_user_id ON zip_downloads(user_id);
CREATE INDEX IF NOT EXISTS idx_zip_downloads_status ON zip_downloads(status);
CREATE INDEX IF NOT EXISTS idx_zip_downloads_expires_at ON zip_downloads(expires_at) WHERE expires_at IS NOT NULL;

-- ============================================================================
-- WEBHOOK DEAD LETTER QUEUE
-- ============================================================================

-- Webhook dead letter queue for permanently failed deliveries
CREATE TABLE IF NOT EXISTS webhook_dlq (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    webhook_id UUID NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    delivery_id UUID REFERENCES webhook_deliveries(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    final_error TEXT NOT NULL,
    attempts INTEGER NOT NULL,
    last_response_code INTEGER,
    last_response_body TEXT,
    can_retry BOOLEAN NOT NULL DEFAULT true,
    retried_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webhook_dlq_webhook_id ON webhook_dlq(webhook_id);
CREATE INDEX IF NOT EXISTS idx_webhook_dlq_can_retry ON webhook_dlq(webhook_id) WHERE can_retry = true;
CREATE INDEX IF NOT EXISTS idx_webhook_dlq_created_at ON webhook_dlq(created_at DESC);
