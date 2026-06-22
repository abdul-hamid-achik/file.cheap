// Package db provides the SQLite database layer for stash metadata.
// It uses modernc.org/sqlite (pure Go, no CGO).
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite database for stash metadata.
type Store struct {
	db *sql.DB
}

// Open creates or opens a database at the given path.
// It runs migrations if the database is new.
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dsn := "file:" + dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("ping database: %w", err)
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return store, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for advanced use.
func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate() error {
	schema := `
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
`
	_, err := s.db.Exec(schema)
	return err
}

// StashRecord is a row in the stashes table.
type StashRecord struct {
	ID             string
	Name           string
	SourcePath     string
	Tool           string
	CreatedAt      string
	FileCount      int
	TotalSize      int64
	ContentHash    string
	Compression    string
	CompressedSize int64
	BundleType     string
	Indexed        int
}

// CreateStash inserts a new stash record.
func (s *Store) CreateStash(r *StashRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO stashes (id, name, source_path, tool, created_at, file_count, total_size, content_hash, compression, compressed_size, bundle_type, indexed)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.SourcePath, r.Tool, r.CreatedAt,
		r.FileCount, r.TotalSize, r.ContentHash,
		r.Compression, r.CompressedSize, r.BundleType, r.Indexed,
	)
	return err
}

// AddTag adds a tag to a stash.
func (s *Store) AddTag(stashID, tag string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO tags (stash_id, tag) VALUES (?, ?)`, stashID, tag)
	return err
}

// GetStash retrieves a stash by ID.
func (s *Store) GetStash(id string) (*StashRecord, error) {
	row := s.db.QueryRow(`SELECT id, name, source_path, tool, created_at, file_count, total_size, content_hash, compression, compressed_size, bundle_type, indexed FROM stashes WHERE id = ?`, id)
	var r StashRecord
	err := row.Scan(&r.ID, &r.Name, &r.SourcePath, &r.Tool, &r.CreatedAt, &r.FileCount, &r.TotalSize, &r.ContentHash, &r.Compression, &r.CompressedSize, &r.BundleType, &r.Indexed)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListStashes returns all stashes ordered by creation date descending.
func (s *Store) ListStashes() ([]*StashRecord, error) {
	rows, err := s.db.Query(`SELECT id, name, source_path, tool, created_at, file_count, total_size, content_hash, compression, compressed_size, bundle_type, indexed FROM stashes ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	return scanStashRows(rows)
}

// ListStashesByTag returns stashes that have the given tag.
func (s *Store) ListStashesByTag(tag string) ([]*StashRecord, error) {
	rows, err := s.db.Query(
		`SELECT s.id, s.name, s.source_path, s.tool, s.created_at, s.file_count, s.total_size, s.content_hash, s.compression, s.compressed_size, s.bundle_type, s.indexed
		 FROM stashes s JOIN tags t ON s.id = t.stash_id WHERE t.tag = ? ORDER BY s.created_at DESC`,
		tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	return scanStashRows(rows)
}

// GetStashTags returns all tags for a stash.
func (s *Store) GetStashTags(stashID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT tag FROM tags WHERE stash_id = ?`, stashID)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// DeleteStash removes a stash and its tags.
func (s *Store) DeleteStash(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM tags WHERE stash_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM stashes WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// MarkIndexed sets the indexed flag for a stash.
func (s *Store) MarkIndexed(id string) error {
	_, err := s.db.Exec(`UPDATE stashes SET indexed = 1 WHERE id = ?`, id)
	return err
}

// UpdateCompression updates compression info for a stash.
func (s *Store) UpdateCompression(id, compression string, compressedSize int64) error {
	_, err := s.db.Exec(`UPDATE stashes SET compression = ?, compressed_size = ? WHERE id = ?`, compression, compressedSize, id)
	return err
}

// SearchStashes searches stash metadata by name, tool, or ID.
func (s *Store) SearchStashes(pattern string) ([]*StashRecord, error) {
	like := "%" + pattern + "%"
	rows, err := s.db.Query(
		`SELECT id, name, source_path, tool, created_at, file_count, total_size, content_hash, compression, compressed_size, bundle_type, indexed
		 FROM stashes WHERE name LIKE ? OR tool LIKE ? OR id LIKE ? ORDER BY created_at DESC`,
		like, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	return scanStashRows(rows)
}

func scanStashRows(rows *sql.Rows) ([]*StashRecord, error) {
	var records []*StashRecord
	for rows.Next() {
		var r StashRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.SourcePath, &r.Tool, &r.CreatedAt, &r.FileCount, &r.TotalSize, &r.ContentHash, &r.Compression, &r.CompressedSize, &r.BundleType, &r.Indexed); err != nil {
			return nil, err
		}
		records = append(records, &r)
	}
	return records, rows.Err()
}