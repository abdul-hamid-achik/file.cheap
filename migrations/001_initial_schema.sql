-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================================
-- ENUMS
-- ============================================================================

-- File status enum
CREATE TYPE file_status AS ENUM ('pending', 'processing', 'completed', 'failed');

-- Job type enum
CREATE TYPE job_type AS ENUM ('thumbnail', 'resize');

-- Job status enum  
CREATE TYPE job_status AS ENUM ('pending', 'running', 'completed', 'failed');

-- File variant type enum
CREATE TYPE variant_type AS ENUM ('thumbnail', 'large', 'medium', 'small');

-- ============================================================================
-- TABLES
-- ============================================================================

-- Files table
CREATE TABLE files (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL,
    filename VARCHAR(255) NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL,
    storage_key VARCHAR(500) NOT NULL,
    status file_status NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Processing Jobs table
CREATE TABLE processing_jobs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_id UUID NOT NULL,
    job_type job_type NOT NULL,
    status job_status NOT NULL DEFAULT 'pending',
    priority INTEGER NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    
    -- Foreign key constraint
    CONSTRAINT fk_processing_jobs_file_id
        FOREIGN KEY (file_id)
        REFERENCES files(id)
        ON DELETE CASCADE
);

-- File Variants table
CREATE TABLE file_variants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    file_id UUID NOT NULL,
    variant_type variant_type NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL,
    storage_key VARCHAR(500) NOT NULL,
    width INTEGER,
    height INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Foreign key constraint
    CONSTRAINT fk_file_variants_file_id
        FOREIGN KEY (file_id)
        REFERENCES files(id)
        ON DELETE CASCADE
);

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Files indexes (for common queries, excluding soft-deleted)
CREATE INDEX idx_files_user_id ON files(user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_files_status ON files(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_files_created_at ON files(created_at DESC) WHERE deleted_at IS NULL;

-- Processing jobs indexes (for queue queries)
CREATE INDEX idx_processing_jobs_file_id ON processing_jobs(file_id);
CREATE INDEX idx_processing_jobs_status ON processing_jobs(status);
CREATE INDEX idx_processing_jobs_pending ON processing_jobs(priority DESC, created_at ASC) 
    WHERE status = 'pending';

-- File variants indexes (for listing variants by file)
CREATE INDEX idx_file_variants_file_id ON file_variants(file_id);
CREATE INDEX idx_file_variants_type ON file_variants(file_id, variant_type);
