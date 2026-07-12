// Package stash implements the core stash operations: Save, Restore, Drop, List, Info.
package stash

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/compress"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/detect"
	"github.com/abdul-hamid-achik/file.cheap/internal/fslock"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/file.cheap/internal/secrets"
)

// Options for saving a stash.
type SaveOptions struct {
	SourcePath string            // path to the source file or directory
	Name       string            // optional display name
	Tags       []string          // tags for categorization
	Tool       string            // tool that produced the content (e.g., "vidtrace")
	TTL        string            // optional time-to-live, e.g. "7d", "24h"; empty = never expires
	Custom     map[string]string // agent-provided metadata
	NoScan     bool              // skip the secret scan
}

// Stash represents a saved snapshot with its manifest.
type Stash struct {
	Manifest *manifest.Manifest
	Dir      string            // path to the stash directory on disk
	Secrets  []secrets.Finding // likely-secret findings from the save-time scan
}

// Manager provides stash operations against a root directory.
type Manager struct {
	rootDir string
}

// NewManager creates a stash Manager rooted at the given directory.
// The directory is created if it doesn't exist.
func NewManager(rootDir string) (*Manager, error) {
	if strings.TrimSpace(rootDir) == "" {
		return nil, fmt.Errorf("stash root is required")
	}
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve stash root: %w", err)
	}
	if err := os.MkdirAll(absRoot, 0700); err != nil {
		return nil, fmt.Errorf("create stash root: %w", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve stash root symlinks: %w", err)
	}
	info, err := os.Stat(canonicalRoot)
	if err != nil {
		return nil, fmt.Errorf("stat stash root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("stash root %q is not a directory", rootDir)
	}
	// Stashes frequently contain source code, credentials, and agent artifacts.
	// Keep the vault private even when the directory was created previously with
	// a more permissive mode. Chmod follows a configured root symlink, while the
	// canonical path retained by Manager prevents a later symlink swap.
	if err := os.Chmod(canonicalRoot, 0700); err != nil {
		return nil, fmt.Errorf("secure stash root: %w", err)
	}
	return &Manager{rootDir: canonicalRoot}, nil
}

// RootDir returns the stash root directory.
func (m *Manager) RootDir() string {
	return m.rootDir
}

// dbPath returns the path to the SQLite metadata index.
func (m *Manager) dbPath() string {
	return filepath.Join(m.rootDir, "fcheap.db")
}

// openStore opens the metadata index best-effort. Callers must Close the store
// if ok is true. When the index cannot be opened the stash layer falls back to
// scanning manifests directly, so this never blocks an operation.
func (m *Manager) openStore() (*db.Store, bool) {
	store, err := db.Open(m.dbPath())
	if err != nil {
		return nil, false
	}
	return store, true
}

// syncToDB upserts a manifest into the derived metadata index. Failure does not
// invalidate a successfully persisted stash, but it must be visible so doctor
// and operators are not left with a silently stale catalog.
func (m *Manager) syncToDB(ctx context.Context, man *manifest.Manifest) {
	store, err := db.Open(m.dbPath())
	if err != nil {
		slog.Warn("metadata index unavailable; manifest remains authoritative", "id", man.ID, "err", err)
		return
	}
	defer store.Close() //nolint:errcheck
	if err := store.Sync(ctx, recordFromManifest(man), man.Tags); err != nil {
		slog.Warn("metadata index sync failed; manifest remains authoritative", "id", man.ID, "err", err)
	}
}

// recordFromManifest projects a manifest onto a database row.
func recordFromManifest(man *manifest.Manifest) db.Record {
	var indexed int64
	if man.Custom["indexed"] == "true" {
		indexed = 1
	}
	return db.Record{
		ID:             man.ID,
		Name:           man.Name,
		SourcePath:     man.SourcePath,
		Tool:           man.Tool,
		CreatedAt:      man.CreatedAt,
		FileCount:      int64(man.FileCount),
		TotalSize:      man.TotalSize,
		ContentHash:    man.ContentHash,
		Compression:    man.Compression,
		CompressedSize: man.CompressedSize,
		BundleType:     man.BundleType,
		ExpiresAt:      man.ExpiresAt,
		Indexed:        indexed,
	}
}

// Stats returns the number of stashes and their total logical size. Manifests
// are the source of truth; scanning them also self-heals the derived DB before
// reporting health, avoiding stale rows being presented as valid vault state.
func (m *Manager) Stats(ctx context.Context) (count int, totalSize int64) {
	stashes, err := m.ListFiltered(ctx, ListOptions{IncludeExpired: true})
	if err != nil {
		return 0, 0
	}
	for _, st := range stashes {
		totalSize += st.Manifest.TotalSize
	}
	return len(stashes), totalSize
}

// StashDir returns the directory for a given stash ID.
func (m *Manager) StashDir(id string) string {
	return filepath.Join(m.rootDir, id)
}

// validStashID reports whether id is a safe, single-element stash ID. Stash IDs
// are generated as one directory name, so any id with path separators or
// traversal is invalid input — and, via the MCP server, a path-traversal attempt
// (e.g. "../../etc" would otherwise escape the stash root in StashDir, then be
// passed to os.RemoveAll/Restore). All id-taking Manager methods reject it.
func validStashID(id string) bool {
	if id == "" || id == "." || id == ".." || id != filepath.Base(id) {
		return false
	}
	// These names belong to vault-wide derived indexes, never individual
	// stashes. Rejecting them in the Manager boundary prevents a direct MCP Drop
	// call from deleting SQLite/veclite storage as though it were a stash.
	reservedName := strings.ToLower(id)
	if reservedName == "fcheap.db" || strings.HasPrefix(reservedName, "fcheap.db-") ||
		reservedName == "fcheap.veclite" || strings.HasPrefix(reservedName, "fcheap.veclite.") {
		return false
	}
	return true
}

// loadManifestForID is the single trust boundary for an existing stash. The
// stash directory and manifest must both be real filesystem objects rather
// than symlinks, and the identity in the manifest must match the directory.
// This prevents a planted <vault>/<id> symlink from redirecting reads and
// destructive operations outside the vault.
func (m *Manager) loadManifestForID(id string) (*manifest.Manifest, error) {
	if !validStashID(id) {
		return nil, fmt.Errorf("invalid stash id %q", id)
	}
	stashDir := m.StashDir(id)
	info, err := os.Lstat(stashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperror.ErrStashNotFound
		}
		return nil, fmt.Errorf("lstat stash: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("stash path %q is not a real directory", stashDir)
	}
	manifestPath := filepath.Join(stashDir, "manifest.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("lstat manifest: %w", err)
	}
	if manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("stash manifest %q is not a regular file", manifestPath)
	}
	man, err := manifest.Load(stashDir)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}
	if man.ID != id {
		return nil, fmt.Errorf("stash manifest ID %q does not match directory %q", man.ID, id)
	}
	return man, nil
}

// Save creates a stash from a source path.
// It copies the source file or directory into the stash, creates a manifest,
// and optionally compresses.
func (m *Manager) Save(ctx context.Context, opts *SaveOptions) (*Stash, error) {
	if opts == nil || opts.SourcePath == "" {
		return nil, apperror.New("invalid_input", "source path is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Save enriches Custom with detected metadata. Clone caller-owned slices and
	// maps so concurrent saves that reuse immutable options do not race or leak
	// generated fields back into the caller's object.
	owned := *opts
	owned.Tags = append([]string(nil), opts.Tags...)
	if opts.Custom != nil {
		owned.Custom = make(map[string]string, len(opts.Custom))
		for key, value := range opts.Custom {
			owned.Custom[key] = value
		}
	}
	opts = &owned

	// Reject a symlink as the source root. Nested symlinks are preserved, but a
	// root symlink makes both snapshot semantics and vault-overlap checks
	// ambiguous (filepath.Walk observes the link rather than its directory).
	srcLInfo, err := os.Lstat(opts.SourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperror.Wrap(err, apperror.ErrFileNotFound)
		}
		return nil, fmt.Errorf("lstat source: %w", err)
	}
	if srcLInfo.Mode()&os.ModeSymlink != 0 {
		return nil, apperror.New("invalid_input", "source path must not be a symbolic link; use its resolved path")
	}
	if !srcLInfo.IsDir() && !srcLInfo.Mode().IsRegular() {
		return nil, apperror.New("invalid_input", fmt.Sprintf("unsupported source file type %s", srcLInfo.Mode().Type()))
	}

	canonicalSource, err := filepath.EvalSymlinks(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("resolve source path: %w", err)
	}
	canonicalSource, err = filepath.Abs(canonicalSource)
	if err != nil {
		return nil, fmt.Errorf("resolve absolute source path: %w", err)
	}
	if pathsOverlap(canonicalSource, m.rootDir) {
		return nil, apperror.New(
			"invalid_input",
			"source path must be outside the stash root and must not contain the stash root",
		)
	}

	srcInfo, err := os.Stat(canonicalSource)
	if err != nil {
		return nil, fmt.Errorf("stat resolved source: %w", err)
	}

	id, stashDir, err := m.reserveStashDir(opts.Name, canonicalSource)
	if err != nil {
		return nil, err
	}

	// Copy files into stash
	contentDir := filepath.Join(stashDir, "content")
	if err := os.Mkdir(contentDir, 0700); err != nil {
		_ = os.RemoveAll(stashDir)
		return nil, fmt.Errorf("create content dir: %w", err)
	}

	if srcInfo.IsDir() {
		if err := copyDir(ctx, canonicalSource, contentDir, false); err != nil {
			_ = os.RemoveAll(stashDir)
			return nil, fmt.Errorf("copy directory: %w", err)
		}
	} else {
		filename := filepath.Base(canonicalSource)
		dst := filepath.Join(contentDir, filename)
		if err := copyFile(ctx, canonicalSource, dst); err != nil {
			_ = os.RemoveAll(stashDir)
			return nil, fmt.Errorf("copy file: %w", err)
		}
	}

	// Build manifest
	man := manifest.New(id, opts.SourcePath)
	man.Name = opts.Name
	man.Tool = opts.Tool
	man.Tags = opts.Tags
	man.Custom = opts.Custom

	// Optional TTL: durations are relative to CreatedAt; calendar dates are
	// absolute expiry timestamps.
	// Parsed before ScanFiles so an invalid TTL aborts early (before copying
	// would be wasted, though copy already happened above — at least we fail
	// before the manifest is written and the stash is committed).
	if opts.TTL != "" {
		created, _ := time.Parse(time.RFC3339, man.CreatedAt)
		expiresAt, err := resolveExpiry(opts.TTL, created)
		if err != nil {
			_ = os.RemoveAll(stashDir)
			return nil, fmt.Errorf("invalid ttl: %w", err)
		}
		man.ExpiresAt = expiresAt.UTC().Format(time.RFC3339)
	}

	if err := man.ScanFilesContext(ctx, contentDir); err != nil {
		_ = os.RemoveAll(stashDir)
		return nil, fmt.Errorf("scan files: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = os.RemoveAll(stashDir)
		return nil, err
	}

	// Detect bundle type and, for vidtrace bundles, record key metadata fields
	// (source video, duration, frame rate) onto the manifest for `info`/Studio.
	bt := detect.BundleTypeOf(contentDir)
	man.BundleType = string(bt)
	if bt == detect.TypeVidtrace {
		if meta, ok := detect.VidtraceMetadata(contentDir); ok {
			if man.Custom == nil {
				man.Custom = make(map[string]string)
			}
			if v, ok := meta["source_video"].(string); ok && v != "" {
				man.Custom["source_video"] = v
			}
			if v, ok := meta["duration_seconds"].(float64); ok && v > 0 {
				man.Custom["duration_seconds"] = fmt.Sprintf("%.0f", v)
			}
			if v, ok := meta["frame_rate"].(float64); ok && v > 0 {
				man.Custom["frame_rate"] = fmt.Sprintf("%.0f", v)
			}
		}
	}

	// Scan for likely secrets so the caller can warn before this stash is
	// shared or restored elsewhere. Recorded in the manifest; never the values.
	var findings []secrets.Finding
	if !opts.NoScan {
		findings, err = secrets.ScanContext(ctx, contentDir)
		if err != nil {
			_ = os.RemoveAll(stashDir)
			return nil, fmt.Errorf("scan for secrets: %w", err)
		}
		if len(findings) > 0 {
			if man.Custom == nil {
				man.Custom = make(map[string]string)
			}
			man.Custom["secrets_found"] = fmt.Sprintf("%d", len(findings))
			man.Custom["secrets_rules"] = strings.Join(secrets.Rules(findings), ",")
		}
	}
	if err := ctx.Err(); err != nil {
		_ = os.RemoveAll(stashDir)
		return nil, err
	}

	if err := man.Save(stashDir); err != nil {
		_ = os.RemoveAll(stashDir)
		return nil, fmt.Errorf("save manifest: %w", err)
	}

	m.syncToDB(ctx, man)

	slog.Debug("stash saved", "id", id, "files", man.FileCount, "size", man.TotalSize,
		"bundle", man.BundleType, "secrets", len(findings))
	return &Stash{Manifest: man, Dir: stashDir, Secrets: findings}, nil
}

// RestoreResult reports the outcome of a restore, including hash verification
// of the restored files against the manifest.
type RestoreResult struct {
	Target     string   `json:"target"`
	FileCount  int      `json:"file_count"`
	Verified   bool     `json:"verified"`
	Mismatches []string `json:"mismatches"`
}

// Restore extracts a stash to the given target directory and verifies the
// restored files against the manifest. If target is empty, a temp directory is
// used. The returned result reports whether every manifest file was restored
// with a matching content hash.
func (m *Manager) Restore(ctx context.Context, id, target string) (*RestoreResult, error) {
	if !validStashID(id) {
		return nil, fmt.Errorf("invalid stash id %q", id)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stashDir := m.StashDir(id)
	if _, err := m.loadManifestForID(id); err != nil {
		if apperror.Is(err, apperror.ErrStashNotFound) {
			return nil, err
		}
		return nil, apperror.WrapWithMessage(err, "restore_failed", "stash manifest is invalid")
	}
	operationLock, err := fslock.Acquire(ctx, filepath.Join(stashDir, ".fcheap.lock"))
	if err != nil {
		return nil, err
	}
	defer operationLock.Release() //nolint:errcheck
	man, err := m.loadManifestForID(id)
	if err != nil {
		return nil, apperror.WrapWithMessage(err, "restore_failed", "stash manifest is invalid")
	}

	contentDir := filepath.Join(stashDir, "content")

	if target == "" {
		// Use a fresh, unpredictable temp directory rather than a shared
		// os.TempDir()/<id> path. The predictable path let an attacker pre-plant
		// symlinks (or stale files) at known destinations before a restore.
		dir, err := os.MkdirTemp("", id+"-*")
		if err != nil {
			return nil, fmt.Errorf("create temp target dir: %w", err)
		}
		target = dir
		if err := m.rejectVaultTarget(target); err != nil {
			_ = os.RemoveAll(target)
			return nil, err
		}
	} else {
		if err := m.rejectVaultTarget(target); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(target, 0755); err != nil {
			return nil, fmt.Errorf("create target dir: %w", err)
		}
		// Resolve again after creation so a path whose missing suffix was created
		// cannot evade the relationship check through an existing symlink parent.
		if err := m.rejectVaultTarget(target); err != nil {
			return nil, err
		}
	}

	if dirExists(contentDir) {
		if err := copyDir(ctx, contentDir, target, true); err != nil {
			return nil, apperror.WrapWithMessage(err, "restore_failed", "failed to copy content")
		}
	} else if archivePath, ok := findArchive(stashDir); ok {
		if err := extractArchive(ctx, archivePath, target); err != nil {
			return nil, apperror.WrapWithMessage(err, "restore_failed", "failed to extract archive")
		}
	} else {
		return nil, apperror.New("restore_failed", "no content or archive found in stash")
	}

	res := &RestoreResult{Target: target, FileCount: man.FileCount}
	res.Verified, res.Mismatches, err = verifyRestore(ctx, target, man)
	if err != nil {
		return nil, apperror.WrapWithMessage(err, "restore_failed", "restore verification was interrupted")
	}
	return res, nil
}

// Drop removes a stash entirely, including its metadata index row.
func (m *Manager) Drop(ctx context.Context, id string) error {
	if !validStashID(id) {
		return fmt.Errorf("invalid stash id %q", id)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stashDir := m.StashDir(id)
	stashInfo, err := os.Lstat(stashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return apperror.ErrStashNotFound
		}
		return fmt.Errorf("lstat stash: %w", err)
	}
	if stashInfo.Mode()&os.ModeSymlink == 0 && !stashInfo.IsDir() {
		return fmt.Errorf("stash path %q is not a directory", stashDir)
	}
	if stashInfo.Mode()&os.ModeSymlink == 0 && stashInfo.IsDir() {
		operationLock, err := fslock.Acquire(ctx, filepath.Join(stashDir, ".fcheap.lock"))
		if err != nil {
			return err
		}
		defer operationLock.Release() //nolint:errcheck
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(stashDir); err != nil {
		return fmt.Errorf("remove stash: %w", err)
	}
	if store, ok := m.openStore(); ok {
		defer store.Close() //nolint:errcheck
		if err := store.Delete(ctx, id); err != nil {
			return fmt.Errorf("remove stash metadata: %w", err)
		}
	} else {
		slog.Warn("stash content removed but metadata index was unavailable", "id", id)
	}
	slog.Debug("stash dropped", "id", id)
	return nil
}

// ListOptions filters and bounds a List query. Zero values mean "no filter".
//
// Tag filtering is AND semantics: a stash must contain every listed tag to
// match. This lets callers narrow to a precise intersection, e.g. the codemap
// per-branch index cache lists with both `codemap-index` and `repo:<hash>`.
// `Tag` (single) is kept for backward compatibility and merged with `Tags`.
type ListOptions struct {
	Tag            string
	Tags           []string
	Tool           string
	Since          time.Time // only stashes created at/after this time
	Limit          int       // 0 = unlimited
	IncludeExpired bool      // include expired stashes (default: hide them)
}

// List returns all stashes, optionally filtered by tag.
func (m *Manager) List(ctx context.Context, tag string) ([]*Stash, error) {
	return m.ListFiltered(ctx, ListOptions{Tag: tag})
}

// hasAllTags reports whether man contains every tag in the union of tag and
// tags. An empty set matches (no filter). Used for the AND tag filter in
// ListFiltered.
func hasAllTags(man *manifest.Manifest, tag string, tags []string) bool {
	if tag != "" && !man.HasTag(tag) {
		return false
	}
	for _, t := range tags {
		if !man.HasTag(t) {
			return false
		}
	}
	return true
}

// ListFiltered returns stashes matching the given options, newest first. The
// metadata index is kept in sync as a side effect (write-through).
func (m *Manager) ListFiltered(ctx context.Context, opts ListOptions) ([]*Stash, error) {
	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return nil, fmt.Errorf("read stash root: %w", err)
	}

	var stashes []*Stash
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		stashDir := filepath.Join(m.rootDir, entry.Name())
		man, err := m.loadManifestForID(entry.Name())
		if err != nil {
			continue // skip invalid stashes
		}
		// AND tag filter: merge the legacy single Tag with Tags and require
		// the manifest to contain every one of them.
		if !hasAllTags(man, opts.Tag, opts.Tags) {
			continue
		}
		if opts.Tool != "" && man.Tool != opts.Tool {
			continue
		}
		if !opts.Since.IsZero() {
			t, perr := time.Parse(time.RFC3339, man.CreatedAt)
			if perr != nil || t.Before(opts.Since) {
				continue
			}
		}
		// Hide expired stashes unless the caller explicitly opts in. Expired
		// stashes are still on disk — they're cleaned up by SweepExpired, not
		// by hiding them — but listing them by default would be noise.
		if !opts.IncludeExpired && IsExpired(man) {
			continue
		}
		stashes = append(stashes, &Stash{Manifest: man, Dir: stashDir})
	}

	// Newest first.
	sort.Slice(stashes, func(i, j int) bool {
		return stashes[i].Manifest.CreatedAt > stashes[j].Manifest.CreatedAt
	})

	// Write-through: keep the metadata index in sync with what's on disk.
	if store, ok := m.openStore(); ok {
		defer store.Close() //nolint:errcheck
		for _, st := range stashes {
			_ = store.Sync(ctx, recordFromManifest(st.Manifest), st.Manifest.Tags)
		}
	}

	if opts.Limit > 0 && len(stashes) > opts.Limit {
		stashes = stashes[:opts.Limit]
	}
	return stashes, nil
}

// parseDuration resolves a human duration expression to a Go time.Duration.
// It accepts Go durations (24h, 90m), day/week shorthands (7d, 2w). Shared by
// ParseSince (age — subtracted from now) and ParseTTL (future expiry — added
// to now). Dates are NOT handled here because they have different semantics
// for since (absolute cutoff) vs TTL (relative until-date); those callers
// handle dates themselves.
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	if len(s) > 1 {
		switch unit := s[len(s)-1]; unit {
		case 'd', 'w':
			if n, err := strconv.Atoi(s[:len(s)-1]); err == nil {
				mult := 24 * time.Hour
				if unit == 'w' {
					mult = 7 * 24 * time.Hour
				}
				return time.Duration(n) * mult, nil
			}
		}
	}
	return 0, fmt.Errorf("invalid duration %q (use e.g. 24h, 7d, or 2w)", s)
}

// parseDateOrDuration returns either an absolute time (for a date string like
// "2006-01-02") or a duration. This lets ParseSince treat a date as a cutoff
// and ParseTTL treat it as "expire on this date".
func parseDateOrDuration(s string) (time.Duration, time.Time, error) {
	s = strings.TrimSpace(s)
	// Try date first — a date means different things for since vs TTL.
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return 0, t, nil
	}
	d, err := parseDuration(s)
	if err != nil {
		return 0, time.Time{}, err
	}
	return d, time.Time{}, nil
}

// ParseSince resolves an age expression to an absolute cutoff time. It accepts
// Go durations (24h, 90m), day/week shorthands (7d, 2w), and dates (2006-01-02).
// A date is the cutoff itself (stashes older than this date).
func ParseSince(s string) (time.Time, error) {
	d, date, err := parseDateOrDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid since value %q (use e.g. 24h, 7d, 2w, or 2026-06-01)", s)
	}
	if !date.IsZero() {
		return date, nil
	}
	return time.Now().Add(-d), nil
}

// ParseTTL resolves a time-to-live expression to a Go duration. It accepts
// the same shorthands as ParseSince (24h, 7d, 2w). A date (2006-01-02) is
// interpreted as "expire on this date" (the duration from now until that date).
func ParseTTL(s string) (time.Duration, error) {
	d, date, err := parseDateOrDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid ttl %q (use e.g. 24h, 7d, 2w, or 2026-06-01)", s)
	}
	if !date.IsZero() {
		dur := time.Until(date)
		if dur < 0 {
			return 0, fmt.Errorf("ttl date %q is in the past", s)
		}
		return dur, nil
	}
	return d, nil
}

// resolveExpiry preserves the distinction that ParseTTL's duration-only API
// cannot: a calendar date is absolute, while a duration is relative to the
// stash creation time.
func resolveExpiry(s string, created time.Time) (time.Time, error) {
	d, date, err := parseDateOrDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid ttl %q (use e.g. 24h, 7d, 2w, or 2026-06-01)", s)
	}
	if !date.IsZero() {
		if date.Before(time.Now()) {
			return time.Time{}, fmt.Errorf("ttl date %q is in the past", s)
		}
		return date, nil
	}
	return created.Add(d), nil
}

// VacuumResult reports what a vacuum reclaimed.
type VacuumResult struct {
	OnDisk       int      `json:"on_disk"`
	OrphanedRows int      `json:"orphaned_rows"`
	Orphans      []string `json:"orphans,omitempty"`
}

// onDiskIDs returns IDs backed by a parseable, self-consistent manifest. Merely
// having a file named manifest.json is not enough to keep stale DB/search rows
// alive for a corrupt stash.
func (m *Manager) onDiskIDs() []string {
	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		man, err := m.loadManifestForID(e.Name())
		if err == nil {
			ids = append(ids, man.ID)
		}
	}
	return ids
}

// Vacuum removes metadata-index rows (and, via the dropIndex callback, search
// documents) for stashes whose directory no longer exists, then compacts the
// SQLite database. dropIndex may be nil. It is a no-op if no metadata index exists.
func (m *Manager) Vacuum(ctx context.Context, dropIndex func(id string) error) (*VacuumResult, error) {
	onDisk := m.onDiskIDs()
	res := &VacuumResult{OnDisk: len(onDisk)}

	store, ok := m.openStore()
	if !ok {
		return res, nil
	}
	defer store.Close() //nolint:errcheck

	dbIDs, err := store.AllIDs(ctx)
	if err != nil {
		return nil, err
	}
	valid := make(map[string]struct{}, len(onDisk))
	for _, id := range onDisk {
		valid[id] = struct{}{}
	}
	for id := range dbIDs {
		if _, ok := valid[id]; ok {
			continue
		}
		_ = store.Delete(ctx, id)
		if dropIndex != nil {
			_ = dropIndex(id)
		}
		res.Orphans = append(res.Orphans, id)
	}
	res.OrphanedRows = len(res.Orphans)
	sort.Strings(res.Orphans)

	if err := store.Vacuum(ctx); err != nil {
		return nil, fmt.Errorf("vacuum database: %w", err)
	}
	return res, nil
}

// CompressResult reports the outcome of a compress operation.
type CompressResult struct {
	ID             string `json:"id"`
	Algorithm      string `json:"algorithm"`
	ArchivePath    string `json:"archive_path"`
	OriginalSize   int64  `json:"original_size"`
	CompressedSize int64  `json:"compressed_size"`
}

// Compress archives a stash's extracted content into a single compressed file
// and removes the extracted tree to reclaim disk space. If the stash is already
// compressed, it is transparently re-compressed with the requested algorithm.
// The manifest is updated with the compression algorithm and on-disk size.
func (m *Manager) Compress(ctx context.Context, id, algo string) (*CompressResult, error) {
	if !validStashID(id) {
		return nil, apperror.New("invalid_input", fmt.Sprintf("invalid stash id %q", id))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stashDir := m.StashDir(id)
	if algo == "" {
		algo = "zstd"
	}
	name, err := archiveName(algo)
	if err != nil {
		return nil, err
	}

	if _, err := m.loadManifestForID(id); err != nil {
		return nil, err
	}
	operationLock, err := fslock.Acquire(ctx, filepath.Join(stashDir, ".fcheap.lock"))
	if err != nil {
		return nil, err
	}
	defer operationLock.Release() //nolint:errcheck
	man, err := m.loadManifestForID(id)
	if err != nil {
		return nil, err
	}

	contentDir := filepath.Join(stashDir, "content")
	var cleanupTmp string
	if !dirExists(contentDir) {
		// Already compressed — extract into a temp tree so we can re-archive.
		existing, ok := findArchive(stashDir)
		if !ok {
			return nil, apperror.New("compress_failed", "stash has no content to compress")
		}
		tmp := filepath.Join(stashDir, ".recompress")
		_ = os.RemoveAll(tmp)
		if err := extractArchive(ctx, existing, tmp); err != nil {
			return nil, fmt.Errorf("extract for recompress: %w", err)
		}
		contentDir = tmp
		cleanupTmp = tmp
	}

	archivePath := filepath.Join(stashDir, name)
	tmpArchive := archivePath + ".tmp"
	if _, err := compress.ArchiveContext(ctx, contentDir, tmpArchive, compress.Algorithm(algo)); err != nil {
		if cleanupTmp != "" {
			_ = os.RemoveAll(cleanupTmp)
		}
		_ = os.Remove(tmpArchive)
		return nil, fmt.Errorf("archive: %w", err)
	}
	if err := ctx.Err(); err != nil {
		if cleanupTmp != "" {
			_ = os.RemoveAll(cleanupTmp)
		}
		_ = os.Remove(tmpArchive)
		return nil, err
	}

	// Move the new archive into place FIRST — a same-named existing archive is
	// replaced in a single atomic rename — so there is never a crash window with
	// zero archives on disk.
	if err := os.Rename(tmpArchive, archivePath); err != nil {
		if cleanupTmp != "" {
			_ = os.RemoveAll(cleanupTmp)
		}
		_ = os.Remove(tmpArchive)
		return nil, fmt.Errorf("finalize archive: %w", err)
	}

	fi, err := os.Stat(archivePath)
	if err != nil {
		return nil, fmt.Errorf("stat archive: %w", err)
	}
	compressedSize := fi.Size()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Record compression in the manifest (atomically) BEFORE reclaiming the
	// content tree, so a crash never leaves the manifest/DB claiming
	// "uncompressed" while the content tree is already gone.
	man.Compression = algo
	man.CompressedSize = compressedSize
	if err := man.Save(stashDir); err != nil {
		return nil, fmt.Errorf("save manifest: %w", err)
	}
	m.syncToDB(ctx, man)
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Now reclaim space: stale archives of other algorithms, the extracted tree,
	// and any temp extraction. A crash here only leaves reclaimable files behind,
	// not an inconsistent stash.
	for _, other := range archiveNames {
		if other != name {
			_ = os.Remove(filepath.Join(stashDir, other))
		}
	}
	_ = os.RemoveAll(filepath.Join(stashDir, "content"))
	if cleanupTmp != "" {
		_ = os.RemoveAll(cleanupTmp)
	}

	slog.Debug("stash compressed", "id", id, "algo", algo,
		"original", man.TotalSize, "compressed", compressedSize)
	return &CompressResult{
		ID:             id,
		Algorithm:      algo,
		ArchivePath:    archivePath,
		OriginalSize:   man.TotalSize,
		CompressedSize: compressedSize,
	}, nil
}

// Info returns detailed info about a single stash.
func (m *Manager) Info(ctx context.Context, id string) (*Stash, error) {
	if !validStashID(id) {
		return nil, fmt.Errorf("invalid stash id %q", id)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stashDir := m.StashDir(id)
	man, err := m.loadManifestForID(id)
	if err != nil {
		return nil, err
	}
	return &Stash{Manifest: man, Dir: stashDir}, nil
}

// Exists returns true if a stash with the given ID has a readable,
// self-consistent manifest. An invalid (e.g. traversal) ID, a partial save, or
// a corrupt/misplaced manifest is never considered an operational stash. This
// also guards callers that gate on Exists before using StashDir (analyze,
// connect, diff).
func (m *Manager) Exists(id string) bool {
	if !validStashID(id) {
		return false
	}
	_, err := m.loadManifestForID(id)
	return err == nil
}

const (
	idEntropyBytes      = 12
	idTimestampLayout   = "20060102_150405.000000000"
	maxStashIDBytes     = 99
	maxIDSlugBytes      = maxStashIDBytes - len(idTimestampLayout) - 2*idEntropyBytes - 2
	maxIDCreateAttempts = 16
)

var (
	idSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)
	idCounter     atomic.Uint64
)

// generateID creates a bounded, portable stash ID. The readable slug is ASCII
// and capped well below common filesystem component limits; timestamp and
// cryptographic entropy make IDs unique even for concurrent saves with the same
// long or Unicode-only name.
func generateID(name, sourcePath string) string {
	base := name
	if base == "" {
		base = filepath.Base(sourcePath)
	}
	// Slugify: lowercase, replace non-alphanumeric with underscores
	base = strings.ToLower(base)
	base = idSlugPattern.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		base = "stash"
	}
	if len(base) > maxIDSlugBytes {
		base = strings.TrimRight(base[:maxIDSlugBytes], "_")
	}

	now := time.Now().UTC()
	entropy := make([]byte, idEntropyBytes)
	if _, err := rand.Read(entropy); err != nil {
		// crypto/rand failures are exceptionally rare. Retain collision
		// resistance within this process with nanoseconds plus an atomic counter,
		// then hash to the same fixed-width suffix.
		seed := fmt.Sprintf("%d:%d:%s:%s", now.UnixNano(), idCounter.Add(1), name, sourcePath)
		sum := sha256.Sum256([]byte(seed))
		copy(entropy, sum[:idEntropyBytes])
	}
	timestamp := now.Format(idTimestampLayout)
	return fmt.Sprintf("%s_%s_%s", base, timestamp, hex.EncodeToString(entropy))
}

// reserveStashDir atomically claims a new stash directory. os.Mkdir provides
// exclusive creation across goroutines and processes; a new random ID is
// generated on the vanishingly unlikely collision.
func (m *Manager) reserveStashDir(name, sourcePath string) (string, string, error) {
	for range maxIDCreateAttempts {
		id := generateID(name, sourcePath)
		stashDir := m.StashDir(id)
		if err := os.Mkdir(stashDir, 0700); err == nil {
			return id, stashDir, nil
		} else if !os.IsExist(err) {
			return "", "", fmt.Errorf("create stash dir: %w", err)
		}
	}
	return "", "", apperror.ErrStashExists
}

// canonicalPath resolves every existing symlink component in path. It also
// supports a not-yet-created leaf by resolving the nearest existing ancestor
// and appending the clean missing suffix.
func canonicalPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absPath = filepath.Clean(absPath)

	current := absPath
	var missing []string
	for {
		resolved, evalErr := filepath.EvalSymlinks(current)
		if evalErr == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(evalErr) {
			return "", evalErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", evalErr
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

// pathWithin reports whether path is base itself or a descendant of base.
// Inputs must be absolute, canonical paths.
func pathWithin(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func pathsOverlap(a, b string) bool {
	return pathWithin(a, b) || pathWithin(b, a)
}

// rejectVaultTarget prevents a restore from writing into its own backing
// store, including through a symlinked target or symlinked existing parent.
func (m *Manager) rejectVaultTarget(target string) error {
	canonicalTarget, err := canonicalPath(target)
	if err != nil {
		return fmt.Errorf("resolve restore target: %w", err)
	}
	if pathsOverlap(canonicalTarget, m.rootDir) {
		return apperror.New("invalid_input", "restore target must not overlap the stash root")
	}
	return nil
}

// copyDir copies src into dst. When restore is true (restoring a stash into a
// user-chosen target) it refuses symlinks that resolve outside dst, matching
// compress.Extract's policy; when false (saving into the vault) it preserves
// symlinks verbatim so the stash faithfully mirrors the source.
func copyDir(ctx context.Context, src, dst string, restore bool) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("copy source root %q must not be a symbolic link", src)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("copy source root %q is not a directory", src)
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			if restore {
				if err := ensureNoSymlinkDestination(dst, target); err != nil {
					return err
				}
			}
			return os.MkdirAll(target, info.Mode())
		}
		// Recreate symlinks rather than dereferencing them: copyFile uses os.Open,
		// which follows links, so a dangling symlink would abort the whole copy.
		if info.Mode()&os.ModeSymlink != 0 {
			linkDest, lerr := os.Readlink(path)
			if lerr != nil {
				return fmt.Errorf("read symlink %q: %w", path, lerr)
			}
			if restore && symlinkEscapes(dst, target, linkDest) {
				return nil // never materialize an escaping link into the target
			}
			if restore {
				if err := ensureNoSymlinkDestination(dst, filepath.Dir(target)); err != nil {
					return err
				}
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			_ = os.Remove(target) // overwrite if it already exists
			return os.Symlink(linkDest, target)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type at %q: %s", path, info.Mode().Type())
		}
		if restore {
			if err := ensureNoSymlinkDestination(dst, filepath.Dir(target)); err != nil {
				return err
			}
		}
		return copyFile(ctx, path, target)
	})
}

// symlinkEscapes reports whether a symlink at linkPath pointing to linkDest would
// resolve outside base (absolute, or relative-escaping).
func symlinkEscapes(base, linkPath, linkDest string) bool {
	if filepath.IsAbs(linkDest) {
		return true
	}
	resolved := filepath.Join(filepath.Dir(linkPath), linkDest)
	rel, err := filepath.Rel(base, resolved)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ensureNoSymlinkDestination rejects existing symlinks in path components
// below a restore root. Without this check a target like target/sub/file could
// write outside target when target/sub had been planted as a symlink.
func ensureNoSymlinkDestination(base, target string) error {
	rel, err := filepath.Rel(base, target)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("unsafe restore destination %q", target)
	}
	if rel == "." {
		return nil
	}
	current := base
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect restore destination %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to restore through symlink path component %q", current)
		}
	}
	return nil
}

func copyFile(ctx context.Context, src, dst string) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !srcInfo.Mode().IsRegular() {
		return fmt.Errorf("unsupported file type at %q: %s", src, srcInfo.Mode().Type())
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close() //nolint:errcheck

	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}

	// Write a private sibling and rename it over the destination. Besides
	// avoiding partial files, replacement by rename does not follow a planted
	// leaf symlink and breaks any pre-existing hardlink instead of truncating its
	// other names outside the restore target.
	dstFile, err := os.CreateTemp(parent, ".fcheap-copy-*")
	if err != nil {
		return err
	}
	tmpName := dstFile.Name()
	defer os.Remove(tmpName) //nolint:errcheck

	buf := make([]byte, 64*1024)
	if _, err := io.CopyBuffer(dstFile, &contextReader{ctx: ctx, r: srcFile}, buf); err != nil {
		_ = dstFile.Close()
		return err
	}
	if err := dstFile.Chmod(srcInfo.Mode().Perm()); err != nil {
		_ = dstFile.Close()
		return err
	}
	if err := dstFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func fileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

func dirExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir()
}

// archiveNames lists the recognized archive filenames in restore preference order.
var archiveNames = []string{"content.tar.zst", "content.tar.gz", "content.tar"}

// findArchive returns the path to a stash's archive, if one exists.
func findArchive(stashDir string) (string, bool) {
	for _, name := range archiveNames {
		p := filepath.Join(stashDir, name)
		if fileExists(p) {
			return p, true
		}
	}
	return "", false
}

// archiveName maps a compression algorithm to its archive filename.
func archiveName(algo string) (string, error) {
	switch algo {
	case "zstd":
		return "content.tar.zst", nil
	case "gzip":
		return "content.tar.gz", nil
	case "none":
		return "content.tar", nil
	default:
		return "", fmt.Errorf("unknown compression algorithm: %s", algo)
	}
}

// verifyRestore checks every manifest file was restored to target with a
// matching size and content hash. It returns whether all files verified and a
// list of human-readable mismatch descriptions.
func verifyRestore(ctx context.Context, target string, man *manifest.Manifest) (bool, []string, error) {
	mismatches := make([]string, 0)
	for _, f := range man.Files {
		if err := ctx.Err(); err != nil {
			return false, mismatches, err
		}
		p := filepath.Join(target, f.Path)
		info, err := os.Lstat(p)
		if err != nil {
			mismatches = append(mismatches, f.Path+" (missing)")
			continue
		}
		actualSize := info.Size()
		actualHash := ""
		if info.Mode()&os.ModeSymlink != 0 {
			linkDest, err := os.Readlink(p)
			if err != nil {
				mismatches = append(mismatches, f.Path+" (unreadable symlink)")
				continue
			}
			actualSize = int64(len(linkDest))
			sum := sha256.Sum256([]byte("symlink:" + linkDest))
			actualHash = hex.EncodeToString(sum[:])
		} else if !info.Mode().IsRegular() {
			mismatches = append(mismatches, f.Path+" (unsupported file type)")
			continue
		}
		if actualSize != f.Size {
			mismatches = append(mismatches, f.Path+" (size mismatch)")
			continue
		}
		if f.Hash != "" {
			if actualHash == "" {
				actualHash, err = hashFile(ctx, p)
			}
			if err != nil || actualHash != f.Hash {
				mismatches = append(mismatches, f.Path+" (hash mismatch)")
			}
		}
	}
	return len(mismatches) == 0, mismatches, nil
}

// hashFile returns the hex-encoded SHA-256 of a file's contents.
func hashFile(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck
	h := sha256.New()
	if _, err := io.Copy(h, &contextReader{ctx: ctx, r: f}); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractArchive decompresses a stash archive to the target directory.
func extractArchive(ctx context.Context, archivePath, target string) error {
	return compress.ExtractContext(ctx, archivePath, target)
}
