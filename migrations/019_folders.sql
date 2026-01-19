-- +goose Up
-- Create folder hierarchy

CREATE TABLE folders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_id UUID REFERENCES folders(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    path TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, parent_id, name)
);

ALTER TABLE files ADD COLUMN folder_id UUID REFERENCES folders(id) ON DELETE SET NULL;

CREATE INDEX idx_folders_user_parent ON folders(user_id, parent_id);
CREATE INDEX idx_folders_path ON folders(user_id, path);
CREATE INDEX idx_files_folder ON files(folder_id) WHERE folder_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_files_folder;
DROP INDEX IF EXISTS idx_folders_path;
DROP INDEX IF EXISTS idx_folders_user_parent;
ALTER TABLE files DROP COLUMN IF EXISTS folder_id;
DROP TABLE IF EXISTS folders;
