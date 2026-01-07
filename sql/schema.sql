-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================================
-- ENUMS
-- ============================================================================

-- File status enum
CREATE TYPE file_status AS ENUM ('pending', 'processing', 'completed', 'failed');

-- Job type enum
CREATE TYPE job_type AS ENUM ('thumbnail', 'resize', 'webp', 'watermark', 'pdf_thumbnail', 'metadata', 'optimize');

-- Job status enum  
CREATE TYPE job_status AS ENUM ('pending', 'running', 'completed', 'failed');

-- File variant type enum
CREATE TYPE variant_type AS ENUM (
    'thumbnail',
    'sm',
    'md',
    'lg',
    'xl',
    'og',
    'twitter',
    'instagram_square',
    'instagram_portrait',
    'instagram_story',
    'webp',
    'watermarked',
    'optimized',
    'pdf_preview'
);

-- User roles
CREATE TYPE user_role AS ENUM ('user', 'admin');

-- OAuth providers
CREATE TYPE oauth_provider AS ENUM ('google', 'github');

-- Subscription tiers
CREATE TYPE subscription_tier AS ENUM ('free', 'pro', 'enterprise');

-- Subscription status
CREATE TYPE subscription_status AS ENUM ('none', 'trialing', 'active', 'past_due', 'canceled', 'unpaid');

-- ============================================================================
-- TABLES
-- ============================================================================

-- Users table
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255),
    name VARCHAR(255) NOT NULL,
    avatar_url TEXT,
    role user_role NOT NULL DEFAULT 'user',
    subscription_tier subscription_tier NOT NULL DEFAULT 'free',
    stripe_customer_id VARCHAR(255),
    stripe_subscription_id VARCHAR(255),
    subscription_status subscription_status NOT NULL DEFAULT 'none',
    subscription_period_end TIMESTAMPTZ,
    trial_ends_at TIMESTAMPTZ,
    files_limit INTEGER NOT NULL DEFAULT 100,
    max_file_size BIGINT NOT NULL DEFAULT 10485760,
    transformations_count INTEGER NOT NULL DEFAULT 0,
    transformations_limit INTEGER NOT NULL DEFAULT 100,
    transformations_reset_at TIMESTAMPTZ NOT NULL DEFAULT DATE_TRUNC('month', NOW()) + INTERVAL '1 month',
    email_verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Files table
CREATE TABLE files (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
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

-- OAuth accounts (link external providers to users)
CREATE TABLE oauth_accounts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider oauth_provider NOT NULL,
    provider_user_id VARCHAR(255) NOT NULL,
    access_token TEXT,
    refresh_token TEXT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(provider, provider_user_id)
);

-- Sessions (for web UI authentication)
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    user_agent TEXT,
    ip_address INET,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Password reset tokens
CREATE TABLE password_resets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Email verification tokens
CREATE TABLE email_verifications (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- User settings (preferences)
CREATE TABLE user_settings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    email_notifications BOOLEAN NOT NULL DEFAULT true,
    processing_alerts BOOLEAN NOT NULL DEFAULT true,
    marketing_emails BOOLEAN NOT NULL DEFAULT false,
    default_retention_days INTEGER NOT NULL DEFAULT 30,
    auto_delete_originals BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- API tokens (for programmatic access)
CREATE TABLE api_tokens (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    token_prefix VARCHAR(10) NOT NULL,
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Users indexes
CREATE INDEX idx_users_email ON users(email) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_role ON users(role) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_subscription_tier ON users(subscription_tier) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_created_at ON users(created_at DESC) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_users_stripe_customer_id ON users(stripe_customer_id) WHERE stripe_customer_id IS NOT NULL;
CREATE INDEX idx_users_stripe_subscription_id ON users(stripe_subscription_id) WHERE stripe_subscription_id IS NOT NULL;
CREATE INDEX idx_users_subscription_status ON users(subscription_status) WHERE deleted_at IS NULL;

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

-- OAuth accounts indexes
CREATE INDEX idx_oauth_accounts_user_id ON oauth_accounts(user_id);
CREATE INDEX idx_oauth_accounts_provider ON oauth_accounts(provider, provider_user_id);

-- Sessions indexes (for lookup and cleanup)
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);

-- Password resets indexes
CREATE INDEX idx_password_resets_user_id ON password_resets(user_id);
CREATE INDEX idx_password_resets_token_hash ON password_resets(token_hash);
CREATE INDEX idx_password_resets_expires_at ON password_resets(expires_at) WHERE used_at IS NULL;

-- Email verifications indexes
CREATE INDEX idx_email_verifications_user_id ON email_verifications(user_id);
CREATE INDEX idx_email_verifications_token_hash ON email_verifications(token_hash);
CREATE INDEX idx_email_verifications_expires_at ON email_verifications(expires_at) WHERE verified_at IS NULL;

-- User settings indexes
CREATE INDEX idx_user_settings_user_id ON user_settings(user_id);

-- API tokens indexes
CREATE INDEX idx_api_tokens_user_id ON api_tokens(user_id);
CREATE INDEX idx_api_tokens_token_hash ON api_tokens(token_hash);

-- ============================================================================
-- FILE SHARING AND TRANSFORM CACHE
-- ============================================================================

-- File shares table for public CDN-style URLs
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

-- Transform cache for frequently requested transforms
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

-- ============================================================================
-- USAGE TRACKING
-- ============================================================================

-- Monthly usage history table for analytics and billing
CREATE TABLE monthly_usage (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    year_month VARCHAR(7) NOT NULL,
    transformations_count INTEGER NOT NULL DEFAULT 0,
    bytes_processed BIGINT NOT NULL DEFAULT 0,
    files_uploaded INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, year_month)
);

CREATE INDEX idx_monthly_usage_user ON monthly_usage(user_id, year_month DESC);
