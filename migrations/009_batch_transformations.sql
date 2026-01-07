-- Batch status enum
DO $$ BEGIN
    CREATE TYPE batch_status AS ENUM ('pending', 'processing', 'completed', 'failed', 'partial');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- Batch transformation operations
CREATE TABLE IF NOT EXISTS batch_operations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status batch_status NOT NULL DEFAULT 'pending',
    total_files INTEGER NOT NULL DEFAULT 0,
    completed_files INTEGER NOT NULL DEFAULT 0,
    failed_files INTEGER NOT NULL DEFAULT 0,
    presets TEXT[] NOT NULL DEFAULT '{}',
    webp BOOLEAN NOT NULL DEFAULT false,
    quality INTEGER NOT NULL DEFAULT 85,
    watermark TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_batch_operations_user ON batch_operations(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_batch_operations_status ON batch_operations(status) WHERE status IN ('pending', 'processing');

-- Individual file items within a batch
CREATE TABLE IF NOT EXISTS batch_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    batch_id UUID NOT NULL REFERENCES batch_operations(id) ON DELETE CASCADE,
    file_id UUID NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    status batch_status NOT NULL DEFAULT 'pending',
    job_ids TEXT[] NOT NULL DEFAULT '{}',
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_batch_items_batch ON batch_items(batch_id);
CREATE INDEX IF NOT EXISTS idx_batch_items_file ON batch_items(file_id);
CREATE INDEX IF NOT EXISTS idx_batch_items_status ON batch_items(batch_id, status);
