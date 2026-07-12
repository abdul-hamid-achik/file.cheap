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
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/db/gen"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// currentSchemaVersion is stored in SQLite's PRAGMA user_version. Version 1 is
// the v0.26 schema, which added stashes.expires_at to the original metadata
// index schema.
const currentSchemaVersion = 1

var legacyStashColumns = []string{
	"id",
	"name",
	"source_path",
	"tool",
	"created_at",
	"file_count",
	"total_size",
	"content_hash",
	"compression",
	"compressed_size",
	"bundle_type",
	"indexed",
}

var currentStashColumns = []string{
	"id",
	"name",
	"source_path",
	"tool",
	"created_at",
	"file_count",
	"total_size",
	"content_hash",
	"compression",
	"compressed_size",
	"bundle_type",
	"expires_at",
	"indexed",
}

var requiredTagColumns = []string{"stash_id", "tag"}

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

	dsn := "file:" + dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate"
	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := sqldb.Ping(); err != nil {
		sqldb.Close() //nolint:errcheck
		return nil, fmt.Errorf("ping database: %w", err)
	}
	if err := migrateSchema(sqldb); err != nil {
		sqldb.Close() //nolint:errcheck
		return nil, fmt.Errorf("migrate database schema: %w", err)
	}

	return &Store{sqldb: sqldb, q: dbgen.New(sqldb)}, nil
}

// migrateSchema creates a fresh schema or upgrades a pre-v0.26 schema in one
// transaction. DDL and PRAGMA user_version are transactional in SQLite, so a
// failed validation or migration leaves the original database unchanged.
func migrateSchema(sqldb *sql.DB) error {
	ctx := context.Background()
	tx, err := sqldb.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	version, err := schemaVersion(ctx, tx)
	if err != nil {
		return err
	}
	if version > currentSchemaVersion {
		return fmt.Errorf(
			"database schema version %d is newer than supported version %d; upgrade fcheap before opening this database",
			version,
			currentSchemaVersion,
		)
	}

	tables, err := userTables(ctx, tx)
	if err != nil {
		return err
	}

	switch version {
	case 0:
		if len(tables) == 0 {
			if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
				return fmt.Errorf("create schema: %w", err)
			}
		} else {
			if err := validateSchemaTables(ctx, tx, tables, legacyStashColumns); err != nil {
				return err
			}

			columns, err := tableColumns(ctx, tx, "stashes")
			if err != nil {
				return err
			}
			if _, ok := columns["expires_at"]; !ok {
				if _, err := tx.ExecContext(ctx, "ALTER TABLE stashes ADD COLUMN expires_at TEXT NOT NULL DEFAULT ''"); err != nil {
					return fmt.Errorf("upgrade schema from version 0: add stashes.expires_at: %w", err)
				}
			}

			// Re-apply the declarative schema after the column exists. CREATE IF NOT
			// EXISTS makes this idempotent and creates indexes introduced with v1.
			if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
				return fmt.Errorf("upgrade schema from version 0: apply current schema objects: %w", err)
			}
		}
	case currentSchemaVersion:
		// Validate before re-applying schemaSQL so a malformed table produces an
		// actionable missing-column error rather than an opaque index error.
		if err := validateSchemaTables(ctx, tx, tables, currentStashColumns); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
			return fmt.Errorf("ensure current schema objects: %w", err)
		}
	default:
		return fmt.Errorf("unsupported database schema version %d", version)
	}

	tables, err = userTables(ctx, tx)
	if err != nil {
		return err
	}
	if err := validateSchemaTables(ctx, tx, tables, currentStashColumns); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", currentSchemaVersion)); err != nil {
		return fmt.Errorf("set database schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func schemaVersion(ctx context.Context, tx *sql.Tx) (int, error) {
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read database schema version: %w", err)
	}
	return version, nil
}

// userTables returns the non-SQLite tables in the database as a set.
func userTables(ctx context.Context, tx *sql.Tx) (map[string]struct{}, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT name
		FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("inspect database tables: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	tables := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("inspect database tables: %w", err)
		}
		tables[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inspect database tables: %w", err)
	}
	return tables, nil
}

func validateSchemaTables(ctx context.Context, tx *sql.Tx, tables map[string]struct{}, stashColumns []string) error {
	for _, table := range []string{"stashes", "tags"} {
		if _, ok := tables[table]; !ok {
			return incompatibleSchemaError("required table %q is missing (found: %s)", table, strings.Join(mapKeys(tables), ", "))
		}
	}
	if err := validateTableColumns(ctx, tx, "stashes", stashColumns); err != nil {
		return err
	}
	return validateTableColumns(ctx, tx, "tags", requiredTagColumns)
}

func validateTableColumns(ctx context.Context, tx *sql.Tx, table string, required []string) error {
	columns, err := tableColumns(ctx, tx, table)
	if err != nil {
		return err
	}
	var missing []string
	for _, column := range required {
		if _, ok := columns[column]; !ok {
			missing = append(missing, column)
		}
	}
	if len(missing) > 0 {
		return incompatibleSchemaError("table %q is missing required column(s): %s", table, strings.Join(missing, ", "))
	}
	return nil
}

func tableColumns(ctx context.Context, tx *sql.Tx, table string) (map[string]struct{}, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return nil, fmt.Errorf("inspect table %q: %w", table, err)
	}
	defer rows.Close() //nolint:errcheck

	columns := make(map[string]struct{})
	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue any
			primaryKey   int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("inspect table %q: %w", table, err)
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("inspect table %q: %w", table, err)
	}
	return columns, nil
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func incompatibleSchemaError(format string, args ...any) error {
	detail := fmt.Sprintf(format, args...)
	return fmt.Errorf(
		"incompatible fcheap database schema: %s; move or remove fcheap.db to rebuild the metadata index from manifest.json files",
		detail,
	)
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
