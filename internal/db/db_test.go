package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const legacySchemaSQL = `
CREATE TABLE stashes (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL DEFAULT '',
    source_path     TEXT NOT NULL DEFAULT '',
    tool            TEXT NOT NULL DEFAULT '',
    created_at      TEXT NOT NULL,
    file_count      INTEGER NOT NULL DEFAULT 0,
    total_size      INTEGER NOT NULL DEFAULT 0,
    content_hash    TEXT NOT NULL DEFAULT '',
    compression     TEXT NOT NULL DEFAULT '',
    compressed_size INTEGER NOT NULL DEFAULT 0,
    bundle_type     TEXT NOT NULL DEFAULT '',
    indexed         INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE tags (
    stash_id TEXT NOT NULL,
    tag      TEXT NOT NULL,
    PRIMARY KEY (stash_id, tag),
    FOREIGN KEY (stash_id) REFERENCES stashes(id) ON DELETE CASCADE
);

CREATE INDEX idx_tags_tag ON tags(tag);
CREATE INDEX idx_stashes_created ON stashes(created_at DESC);
`

func TestOpenFreshDatabase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fcheap.db")
	store, err := Open(path)
	assert.NoError(t, err)
	if err != nil {
		return
	}
	defer func() { assert.NoError(t, store.Close()) }()

	assert.Equal(t, currentSchemaVersion, databaseVersion(t, store.sqldb))
	assertRequiredColumns(t, store.sqldb, "stashes", currentStashColumns)
	assertRequiredColumns(t, store.sqldb, "tags", requiredTagColumns)
	assert.True(t, indexExists(t, store.sqldb, "idx_stashes_expires"))

	record := Record{
		ID:        "fresh-row",
		Name:      "fresh",
		CreatedAt: "2026-01-01T00:00:00Z",
		ExpiresAt: "2026-01-02T00:00:00Z",
	}
	assert.NoError(t, store.Sync(context.Background(), record, []string{"test"}))
	expired, err := store.ListExpired(context.Background(), "2026-01-03T00:00:00Z")
	assert.NoError(t, err)
	if assert.Len(t, expired, 1) {
		assert.Equal(t, record.ID, expired[0].ID)
	}
}

func TestOpenMigratesLegacyDatabaseWithoutDataLoss(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fcheap.db")
	createLegacyDatabase(t, path)

	store, err := Open(path)
	assert.NoError(t, err)
	if err != nil {
		return
	}
	defer func() { assert.NoError(t, store.Close()) }()

	assert.Equal(t, currentSchemaVersion, databaseVersion(t, store.sqldb))
	assertRequiredColumns(t, store.sqldb, "stashes", currentStashColumns)
	assert.True(t, indexExists(t, store.sqldb, "idx_stashes_expires"))

	row, err := store.q.GetStash(context.Background(), "legacy-row")
	assert.NoError(t, err)
	assert.Equal(t, "legacy stash", row.Name)
	assert.Equal(t, "/tmp/legacy", row.SourcePath)
	assert.Equal(t, int64(3), row.FileCount)
	assert.Equal(t, int64(42), row.TotalSize)
	assert.Equal(t, int64(1), row.Indexed)
	assert.Empty(t, row.ExpiresAt)

	tags, err := store.q.GetStashTags(context.Background(), "legacy-row")
	assert.NoError(t, err)
	assert.Equal(t, []string{"keep"}, tags)

	assert.NoError(t, store.SetExpiry(context.Background(), "legacy-row", "2027-01-01T00:00:00Z"))
	row, err = store.q.GetStash(context.Background(), "legacy-row")
	assert.NoError(t, err)
	assert.Equal(t, "2027-01-01T00:00:00Z", row.ExpiresAt)
}

func TestOpenMigrationIsIdempotent(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fcheap.db")
	createLegacyDatabase(t, path)

	first, err := Open(path)
	assert.NoError(t, err)
	if err != nil {
		return
	}
	assert.NoError(t, first.Close())

	second, err := Open(path)
	assert.NoError(t, err)
	if err != nil {
		return
	}
	defer func() { assert.NoError(t, second.Close()) }()

	assert.Equal(t, currentSchemaVersion, databaseVersion(t, second.sqldb))
	assert.Equal(t, 1, columnCount(t, second.sqldb, "stashes", "expires_at"))
	count, err := second.q.CountStashes(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestOpenAdoptsUnversionedCurrentSchema(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "fcheap.db")
	raw := openRawDatabase(t, path)
	_, err := raw.Exec(schemaSQL)
	assert.NoError(t, err)
	_, err = raw.Exec(`
		INSERT INTO stashes (id, created_at, expires_at)
		VALUES (?, ?, ?)
	`, "current-row", "2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z")
	assert.NoError(t, err)
	assert.Equal(t, 0, databaseVersion(t, raw))
	assert.NoError(t, raw.Close())

	store, err := Open(path)
	assert.NoError(t, err)
	if err != nil {
		return
	}
	defer func() { assert.NoError(t, store.Close()) }()

	assert.Equal(t, currentSchemaVersion, databaseVersion(t, store.sqldb))
	row, err := store.q.GetStash(context.Background(), "current-row")
	assert.NoError(t, err)
	assert.Equal(t, "2026-02-01T00:00:00Z", row.ExpiresAt)
}

func TestOpenRejectsIncompatibleSchema(t *testing.T) {
	t.Parallel()

	t.Run("missing required columns", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "fcheap.db")
		raw := openRawDatabase(t, path)
		_, err := raw.Exec(`
			CREATE TABLE stashes (
				id TEXT PRIMARY KEY,
				created_at TEXT NOT NULL
			);
			CREATE TABLE tags (
				stash_id TEXT NOT NULL,
				tag TEXT NOT NULL,
				PRIMARY KEY (stash_id, tag)
			);
		`)
		assert.NoError(t, err)
		assert.NoError(t, raw.Close())

		store, err := Open(path)
		assert.Nil(t, store)
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "incompatible fcheap database schema")
			assert.Contains(t, err.Error(), `table "stashes" is missing required column(s)`)
			assert.Contains(t, err.Error(), "rebuild the metadata index from manifest.json files")
		}

		raw = openRawDatabase(t, path)
		defer func() { assert.NoError(t, raw.Close()) }()
		assert.Equal(t, 0, databaseVersion(t, raw))
		assert.Equal(t, 0, columnCount(t, raw, "stashes", "expires_at"))
	})

	t.Run("newer schema version", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "fcheap.db")
		raw := openRawDatabase(t, path)
		_, err := raw.Exec(schemaSQL)
		assert.NoError(t, err)
		_, err = raw.Exec("PRAGMA user_version = 99")
		assert.NoError(t, err)
		assert.NoError(t, raw.Close())

		store, err := Open(path)
		assert.Nil(t, store)
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "schema version 99 is newer")
			assert.Contains(t, err.Error(), "upgrade fcheap")
		}
	})
}

func createLegacyDatabase(t *testing.T, path string) {
	t.Helper()

	raw := openRawDatabase(t, path)
	_, err := raw.Exec(legacySchemaSQL)
	if !assert.NoError(t, err) {
		_ = raw.Close()
		return
	}
	_, err = raw.Exec(`
		INSERT INTO stashes (
			id, name, source_path, tool, created_at,
			file_count, total_size, content_hash,
			compression, compressed_size, bundle_type, indexed
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		"legacy-row",
		"legacy stash",
		"/tmp/legacy",
		"vidtrace",
		"2025-01-01T00:00:00Z",
		3,
		42,
		"abc123",
		"",
		0,
		"generic",
		1,
	)
	assert.NoError(t, err)
	_, err = raw.Exec("INSERT INTO tags (stash_id, tag) VALUES (?, ?)", "legacy-row", "keep")
	assert.NoError(t, err)
	assert.Equal(t, 0, databaseVersion(t, raw))
	assert.NoError(t, raw.Close())
}

func openRawDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()

	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	if err := raw.Ping(); err != nil {
		_ = raw.Close()
		t.Fatalf("ping raw database: %v", err)
	}
	return raw
}

type queryRower interface {
	QueryRow(query string, args ...any) *sql.Row
}

func databaseVersion(t *testing.T, db queryRower) int {
	t.Helper()

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read database version: %v", err)
	}
	return version
}

func columnCount(t *testing.T, db queryRower, table, column string) int {
	t.Helper()

	var count int
	query := `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`
	if err := db.QueryRow(query, table, column).Scan(&count); err != nil {
		t.Fatalf("count column %s.%s: %v", table, column, err)
	}
	return count
}

func assertRequiredColumns(t *testing.T, db queryRower, table string, columns []string) {
	t.Helper()

	for _, column := range columns {
		assert.Equal(t, 1, columnCount(t, db, table, column), "missing column %s.%s", table, column)
	}
}

func indexExists(t *testing.T, db queryRower, name string) bool {
	t.Helper()

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_schema WHERE type = 'index' AND name = ?", name).Scan(&count)
	if err != nil {
		t.Fatalf("check index %s: %v", name, err)
	}
	return count == 1
}
