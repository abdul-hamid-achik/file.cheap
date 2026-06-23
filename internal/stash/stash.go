// Package stash implements the core stash operations: Save, Restore, Drop, List, Info.
package stash

import (
	"context"
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
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/compress"
	"github.com/abdul-hamid-achik/file.cheap/internal/db"
	"github.com/abdul-hamid-achik/file.cheap/internal/detect"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/file.cheap/internal/secrets"
)

// Options for saving a stash.
type SaveOptions struct {
	SourcePath string            // path to the source file or directory
	Name       string            // optional display name
	Tags       []string          // tags for categorization
	Tool       string            // tool that produced the content (e.g., "vidtrace")
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
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("create stash root: %w", err)
	}
	return &Manager{rootDir: rootDir}, nil
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

// syncToDB upserts a manifest into the metadata index (best-effort).
func (m *Manager) syncToDB(ctx context.Context, man *manifest.Manifest) {
	store, ok := m.openStore()
	if !ok {
		return
	}
	defer store.Close() //nolint:errcheck
	_ = store.Sync(ctx, recordFromManifest(man), man.Tags)
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
		Indexed:        indexed,
	}
}

// Stats returns the number of stashes and their total logical size, preferring
// the metadata index and falling back to a manifest scan.
func (m *Manager) Stats(ctx context.Context) (count int, totalSize int64) {
	if store, ok := m.openStore(); ok {
		defer store.Close() //nolint:errcheck
		if s, err := store.Stats(ctx); err == nil {
			return int(s.Count), s.TotalSize
		}
	}
	stashes, _ := m.List(ctx, "")
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
	return id != "" && id != "." && id != ".." && id == filepath.Base(id)
}

// Save creates a stash from a source path.
// It copies the source file or directory into the stash, creates a manifest,
// and optionally compresses.
func (m *Manager) Save(ctx context.Context, opts *SaveOptions) (*Stash, error) {
	if opts == nil || opts.SourcePath == "" {
		return nil, apperror.New("invalid_input", "source path is required")
	}

	srcInfo, err := os.Stat(opts.SourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperror.Wrap(err, apperror.ErrFileNotFound)
		}
		return nil, fmt.Errorf("stat source: %w", err)
	}

	id := generateID(opts.Name, opts.SourcePath)
	stashDir := m.StashDir(id)

	if _, err := os.Stat(stashDir); err == nil {
		return nil, apperror.ErrStashExists
	}

	if err := os.MkdirAll(stashDir, 0755); err != nil {
		return nil, fmt.Errorf("create stash dir: %w", err)
	}

	// Copy files into stash
	contentDir := filepath.Join(stashDir, "content")
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		_ = os.RemoveAll(stashDir)
		return nil, fmt.Errorf("create content dir: %w", err)
	}

	if srcInfo.IsDir() {
		if err := copyDir(opts.SourcePath, contentDir); err != nil {
			_ = os.RemoveAll(stashDir)
			return nil, fmt.Errorf("copy directory: %w", err)
		}
	} else {
		filename := filepath.Base(opts.SourcePath)
		dst := filepath.Join(contentDir, filename)
		if err := copyFile(opts.SourcePath, dst); err != nil {
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

	if err := man.ScanFiles(contentDir); err != nil {
		_ = os.RemoveAll(stashDir)
		return nil, fmt.Errorf("scan files: %w", err)
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
		findings = secrets.Scan(contentDir)
		if len(findings) > 0 {
			if man.Custom == nil {
				man.Custom = make(map[string]string)
			}
			man.Custom["secrets_found"] = fmt.Sprintf("%d", len(findings))
			man.Custom["secrets_rules"] = strings.Join(secrets.Rules(findings), ",")
		}
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
	Mismatches []string `json:"mismatches,omitempty"`
}

// Restore extracts a stash to the given target directory and verifies the
// restored files against the manifest. If target is empty, a temp directory is
// used. The returned result reports whether every manifest file was restored
// with a matching content hash.
func (m *Manager) Restore(ctx context.Context, id, target string) (*RestoreResult, error) {
	if !validStashID(id) {
		return nil, fmt.Errorf("invalid stash id %q", id)
	}
	stashDir := m.StashDir(id)
	if _, err := os.Stat(stashDir); err != nil {
		if os.IsNotExist(err) {
			return nil, apperror.ErrStashNotFound
		}
		return nil, fmt.Errorf("stat stash: %w", err)
	}

	contentDir := filepath.Join(stashDir, "content")

	if target == "" {
		target = filepath.Join(os.TempDir(), id)
	}
	if err := os.MkdirAll(target, 0755); err != nil {
		return nil, fmt.Errorf("create target dir: %w", err)
	}

	if dirExists(contentDir) {
		if err := copyDir(contentDir, target); err != nil {
			return nil, apperror.WrapWithMessage(err, "restore_failed", "failed to copy content")
		}
	} else if archivePath, ok := findArchive(stashDir); ok {
		if err := extractArchive(archivePath, target); err != nil {
			return nil, apperror.WrapWithMessage(err, "restore_failed", "failed to extract archive")
		}
	} else {
		return nil, apperror.New("restore_failed", "no content or archive found in stash")
	}

	res := &RestoreResult{Target: target, Verified: true}
	if man, err := manifest.Load(stashDir); err == nil {
		res.FileCount = man.FileCount
		res.Verified, res.Mismatches = verifyRestore(target, man)
	}
	return res, nil
}

// Drop removes a stash entirely, including its metadata index row.
func (m *Manager) Drop(ctx context.Context, id string) error {
	if !validStashID(id) {
		return fmt.Errorf("invalid stash id %q", id)
	}
	stashDir := m.StashDir(id)
	if _, err := os.Stat(stashDir); err != nil {
		if os.IsNotExist(err) {
			return apperror.ErrStashNotFound
		}
		return fmt.Errorf("stat stash: %w", err)
	}
	if err := os.RemoveAll(stashDir); err != nil {
		return err
	}
	if store, ok := m.openStore(); ok {
		defer store.Close() //nolint:errcheck
		_ = store.Delete(ctx, id)
	}
	slog.Debug("stash dropped", "id", id)
	return nil
}

// ListOptions filters and bounds a List query. Zero values mean "no filter".
type ListOptions struct {
	Tag   string
	Tool  string
	Since time.Time // only stashes created at/after this time
	Limit int       // 0 = unlimited
}

// List returns all stashes, optionally filtered by tag.
func (m *Manager) List(ctx context.Context, tag string) ([]*Stash, error) {
	return m.ListFiltered(ctx, ListOptions{Tag: tag})
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
		if !entry.IsDir() {
			continue
		}
		stashDir := filepath.Join(m.rootDir, entry.Name())
		man, err := manifest.Load(stashDir)
		if err != nil {
			continue // skip invalid stashes
		}
		if opts.Tag != "" && !man.HasTag(opts.Tag) {
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

// ParseSince resolves an age expression to an absolute cutoff time. It accepts
// Go durations (24h, 90m), day/week shorthands (7d, 2w), and dates (2006-01-02).
func ParseSince(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	now := time.Now()
	if d, err := time.ParseDuration(s); err == nil {
		return now.Add(-d), nil
	}
	if len(s) > 1 {
		switch unit := s[len(s)-1]; unit {
		case 'd', 'w':
			if n, err := strconv.Atoi(s[:len(s)-1]); err == nil {
				mult := 24 * time.Hour
				if unit == 'w' {
					mult = 7 * 24 * time.Hour
				}
				return now.Add(-time.Duration(n) * mult), nil
			}
		}
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("invalid since value %q (use e.g. 24h, 7d, 2w, or 2026-06-01)", s)
}

// VacuumResult reports what a vacuum reclaimed.
type VacuumResult struct {
	OnDisk       int      `json:"on_disk"`
	OrphanedRows int      `json:"orphaned_rows"`
	Orphans      []string `json:"orphans,omitempty"`
}

// onDiskIDs returns the IDs of stashes that physically exist (have a manifest).
func (m *Manager) onDiskIDs() []string {
	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && fileExists(filepath.Join(m.rootDir, e.Name(), "manifest.json")) {
			ids = append(ids, e.Name())
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
	stashDir := m.StashDir(id)
	if _, err := os.Stat(stashDir); err != nil {
		if os.IsNotExist(err) {
			return nil, apperror.ErrStashNotFound
		}
		return nil, fmt.Errorf("stat stash: %w", err)
	}
	if algo == "" {
		algo = "zstd"
	}
	name, err := archiveName(algo)
	if err != nil {
		return nil, err
	}

	man, err := manifest.Load(stashDir)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
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
		if err := extractArchive(existing, tmp); err != nil {
			return nil, fmt.Errorf("extract for recompress: %w", err)
		}
		contentDir = tmp
		cleanupTmp = tmp
	}

	archivePath := filepath.Join(stashDir, name)
	tmpArchive := archivePath + ".tmp"
	if _, err := compress.Archive(contentDir, tmpArchive, compress.Algorithm(algo)); err != nil {
		if cleanupTmp != "" {
			_ = os.RemoveAll(cleanupTmp)
		}
		_ = os.Remove(tmpArchive)
		return nil, fmt.Errorf("archive: %w", err)
	}

	// Drop any pre-existing archives (possibly a different algorithm), then
	// atomically move the new archive into place.
	for _, name := range archiveNames {
		_ = os.Remove(filepath.Join(stashDir, name))
	}
	if err := os.Rename(tmpArchive, archivePath); err != nil {
		return nil, fmt.Errorf("finalize archive: %w", err)
	}

	fi, err := os.Stat(archivePath)
	if err != nil {
		return nil, fmt.Errorf("stat archive: %w", err)
	}
	compressedSize := fi.Size()

	// Reclaim space: remove the extracted tree and any temp extraction.
	_ = os.RemoveAll(filepath.Join(stashDir, "content"))
	if cleanupTmp != "" {
		_ = os.RemoveAll(cleanupTmp)
	}

	man.Compression = algo
	man.CompressedSize = compressedSize
	if err := man.Save(stashDir); err != nil {
		return nil, fmt.Errorf("save manifest: %w", err)
	}

	m.syncToDB(ctx, man)

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
	stashDir := m.StashDir(id)
	if _, err := os.Stat(stashDir); err != nil {
		if os.IsNotExist(err) {
			return nil, apperror.ErrStashNotFound
		}
		return nil, fmt.Errorf("stat stash: %w", err)
	}
	man, err := manifest.Load(stashDir)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}
	return &Stash{Manifest: man, Dir: stashDir}, nil
}

// Exists returns true if a stash with the given ID exists. An invalid (e.g.
// traversal) id is never considered to exist, which also guards the callers that
// gate on Exists before using StashDir (analyze, connect, diff).
func (m *Manager) Exists(id string) bool {
	if !validStashID(id) {
		return false
	}
	_, err := os.Stat(m.StashDir(id))
	return err == nil
}

// generateID creates a stash ID from name and source path.
func generateID(name, sourcePath string) string {
	base := name
	if base == "" {
		base = filepath.Base(sourcePath)
	}
	// Slugify: lowercase, replace non-alphanumeric with underscores
	base = strings.ToLower(base)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	base = re.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		base = "stash"
	}
	timestamp := time.Now().Format("20060102_150405")
	return fmt.Sprintf("%s_%s", base, timestamp)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		// Recreate symlinks verbatim rather than dereferencing them: copyFile
		// uses os.Open, which follows links, so a dangling symlink would abort
		// the entire save. Preserving the link also keeps the stash faithful.
		if info.Mode()&os.ModeSymlink != 0 {
			linkDest, lerr := os.Readlink(path)
			if lerr != nil {
				return nil // skip links whose metadata can't be read
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			_ = os.Remove(target) // overwrite if it already exists
			return os.Symlink(linkDest, target)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close() //nolint:errcheck

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close() //nolint:errcheck

	buf := make([]byte, 64*1024)
	for {
		n, err := srcFile.Read(buf)
		if n > 0 {
			if _, werr := dstFile.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			break
		}
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
func verifyRestore(target string, man *manifest.Manifest) (bool, []string) {
	var mismatches []string
	for _, f := range man.Files {
		p := filepath.Join(target, f.Path)
		info, err := os.Stat(p)
		if err != nil {
			mismatches = append(mismatches, f.Path+" (missing)")
			continue
		}
		if info.Size() != f.Size {
			mismatches = append(mismatches, f.Path+" (size mismatch)")
			continue
		}
		if f.Hash != "" {
			h, err := hashFile(p)
			if err != nil || h != f.Hash {
				mismatches = append(mismatches, f.Path+" (hash mismatch)")
			}
		}
	}
	return len(mismatches) == 0, mismatches
}

// hashFile returns the hex-encoded SHA-256 of a file's contents.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractArchive decompresses a stash archive to the target directory.
func extractArchive(archivePath, target string) error {
	return compress.Extract(archivePath, target)
}
