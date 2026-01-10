-- Schema migrations tracking table
-- This table records which migrations have been applied to prevent re-running them
-- Must be the first migration (000) to ensure it exists before any tracking happens

CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for faster lookups
CREATE INDEX IF NOT EXISTS idx_schema_migrations_applied_at ON schema_migrations(applied_at);
