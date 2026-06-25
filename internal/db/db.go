// Package db provides the SQLite metadata index for stashes.
//
// It uses modernc.org/sqlite (pure Go, no CGO) and sqlc-generated, type-safe
// queries (see gen/). The manifest.json written alongside each stash remains
// the portable source of truth; this database is a queryable, write-through
// index kept in sync with it. All callers treat it as best-effort: if it fails
// to open, the stash layer falls back to scanning manifests directly.
package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/db/gen"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Record is a single stash row. It is an alias for the sqlc-generated model so
// callers can read fields directly.
type Record = dbgen.Stash

// Store wraps the SQLite database and its generated queries.
type Store struct {
	sqldb *sql.DB
	q     *dbgen.Queries
}

// Open creates or opens the database at the given path and applies the schema.
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dsn := "file:" + dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := sqldb.Ping(); err != nil {
		sqldb.Close() //nolint:errcheck
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if _, err := sqldb.Exec(schemaSQL); err != nil {
		sqldb.Close() //nolint:errcheck
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &Store{sqldb: sqldb, q: dbgen.New(sqldb)}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s == nil || s.sqldb == nil {
		return nil
	}
	return s.sqldb.Close()
}

// Sync upserts a stash row and replaces its tags.
func (s *Store) Sync(ctx context.Context, r Record, tags []string) error {
	if err := s.q.UpsertStash(ctx, dbgen.UpsertStashParams(r)); err != nil {
		return err
	}
	if err := s.q.DeleteTagsForStash(ctx, r.ID); err != nil {
		return err
	}
	for _, t := range tags {
		if err := s.q.AddTag(ctx, dbgen.AddTagParams{StashID: r.ID, Tag: t}); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes a stash and its tags (tags cascade via the foreign key).
func (s *Store) Delete(ctx context.Context, id string) error {
	return s.q.DeleteStash(ctx, id)
}

// SetCompression updates the compression columns for a stash.
func (s *Store) SetCompression(ctx context.Context, id, algo string, compressedSize int64) error {
	return s.q.UpdateCompression(ctx, dbgen.UpdateCompressionParams{
		Compression:    algo,
		CompressedSize: compressedSize,
		ID:             id,
	})
}

// MarkIndexed flags a stash as indexed.
func (s *Store) MarkIndexed(ctx context.Context, id string) error {
	return s.q.MarkIndexed(ctx, id)
}

// SetExpiry updates the expires_at column for a stash.
func (s *Store) SetExpiry(ctx context.Context, id, expiresAt string) error {
	return s.q.UpdateExpiry(ctx, dbgen.UpdateExpiryParams{
		ExpiresAt: expiresAt,
		ID:        id,
	})
}

// ListExpired returns stashes whose expires_at is non-empty and before the
// given cutoff (typically now, formatted as RFC3339).
func (s *Store) ListExpired(ctx context.Context, cutoff string) ([]Record, error) {
	return s.q.ListExpiredStashes(ctx, cutoff)
}

// Vacuum compacts the database file, reclaiming space from deleted rows.
func (s *Store) Vacuum(ctx context.Context) error {
	_, err := s.sqldb.ExecContext(ctx, "VACUUM")
	return err
}

// AllIDs returns the set of stash IDs known to the database.
func (s *Store) AllIDs(ctx context.Context) (map[string]struct{}, error) {
	ids, err := s.q.ListStashIDs(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, nil
}

// Stats holds aggregate metadata for the status/doctor views.
type Stats struct {
	Count     int64
	TotalSize int64
}

// Stats returns the stash count and total logical size.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	count, err := s.q.CountStashes(ctx)
	if err != nil {
		return Stats{}, err
	}
	rows, err := s.q.ListStashes(ctx)
	if err != nil {
		return Stats{}, err
	}
	var total int64
	for _, r := range rows {
		total += r.TotalSize
	}
	return Stats{Count: count, TotalSize: total}, nil
}
