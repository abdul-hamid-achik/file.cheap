-- schema.sql for fcheap stash metadata
-- Used by sqlc to generate type-safe Go code.
-- SQLite (modernc.org/sqlite) — no CGO required.

CREATE TABLE IF NOT EXISTS stashes (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL DEFAULT '',
    source_path TEXT NOT NULL DEFAULT '',
    tool        TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    file_count  INTEGER NOT NULL DEFAULT 0,
    total_size  INTEGER NOT NULL DEFAULT 0,
    content_hash TEXT NOT NULL DEFAULT '',
    compression TEXT NOT NULL DEFAULT '',
    compressed_size INTEGER NOT NULL DEFAULT 0,
    bundle_type TEXT NOT NULL DEFAULT '',
    indexed     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS tags (
    stash_id TEXT NOT NULL,
    tag      TEXT NOT NULL,
    PRIMARY KEY (stash_id, tag),
    FOREIGN KEY (stash_id) REFERENCES stashes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);
CREATE INDEX IF NOT EXISTS idx_stashes_created ON stashes(created_at DESC);