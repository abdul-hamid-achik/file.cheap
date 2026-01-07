-- Add transformation tracking columns to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS transformations_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS transformations_limit INTEGER NOT NULL DEFAULT 100;
ALTER TABLE users ADD COLUMN IF NOT EXISTS transformations_reset_at TIMESTAMPTZ NOT NULL DEFAULT DATE_TRUNC('month', NOW()) + INTERVAL '1 month';

-- Monthly usage history table for analytics and billing
CREATE TABLE IF NOT EXISTS monthly_usage (
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

CREATE INDEX IF NOT EXISTS idx_monthly_usage_user ON monthly_usage(user_id, year_month DESC);

-- Set transformation limits based on current subscription tier
UPDATE users SET transformations_limit = 100 WHERE subscription_tier = 'free';
UPDATE users SET transformations_limit = 10000 WHERE subscription_tier = 'pro';
UPDATE users SET transformations_limit = -1 WHERE subscription_tier = 'enterprise';
