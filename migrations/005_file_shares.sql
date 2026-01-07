-- Migration: 005_file_shares.sql
-- Description: Add file sharing functionality for CDN-style public URLs

CREATE TABLE file_shares (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    token VARCHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ,
    allowed_transforms TEXT[],
    access_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_file_shares_token ON file_shares(token);
CREATE INDEX idx_file_shares_file_id ON file_shares(file_id);
CREATE INDEX idx_file_shares_expires_at ON file_shares(expires_at) WHERE expires_at IS NOT NULL;

-- Add transform cache table for frequently requested transforms
CREATE TABLE transform_cache (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    cache_key VARCHAR(32) NOT NULL,
    transform_params TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL,
    width INT,
    height INT,
    request_count INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(file_id, cache_key)
);

CREATE INDEX idx_transform_cache_file_id ON transform_cache(file_id);
CREATE INDEX idx_transform_cache_lookup ON transform_cache(file_id, cache_key);
CREATE INDEX idx_transform_cache_request_count ON transform_cache(request_count DESC);
