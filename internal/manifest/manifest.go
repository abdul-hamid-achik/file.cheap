// Package manifest defines the metadata structure for a stash snapshot.
package manifest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manifest is the metadata file written alongside each stash.
// It is stored as manifest.json in the stash directory.
type Manifest struct {
	SchemaVersion  string            `json:"schema_version"`
	ID             string            `json:"id"`
	Name           string            `json:"name,omitempty"`
	CreatedAt      string            `json:"created_at"`
	SourcePath     string            `json:"source_path,omitempty"`
	Tool           string            `json:"tool,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	FileCount      int               `json:"file_count"`
	TotalSize      int64             `json:"total_size"`
	ContentHash    string            `json:"content_hash"`
	Compression    string            `json:"compression,omitempty"`
	CompressedSize int64             `json:"compressed_size,omitempty"`
	BundleType     string            `json:"bundle_type,omitempty"`
	ExpiresAt      string            `json:"expires_at,omitempty"` // RFC3339; empty = never expires
	Files          []FileEntry       `json:"files,omitempty"`
	Custom         map[string]string `json:"custom,omitempty"`
}

// FileEntry describes a single file within the stash.
type FileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	Hash string `json:"hash,omitempty"`
}

const SchemaVersion = "1.0"

const maxManifestBytes int64 = 64 << 20 // 64 MiB

// New creates a Manifest from a directory tree.
func New(id, sourcePath string) *Manifest {
	return &Manifest{
		SchemaVersion: SchemaVersion,
		ID:            id,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		SourcePath:    sourcePath,
		Files:         []FileEntry{},
		Tags:          []string{},
		Custom:        map[string]string{},
	}
}

// ScanFiles walks the directory and populates Files, FileCount, TotalSize, and ContentHash.
func (m *Manifest) ScanFiles(dir string) error {
	return m.ScanFilesContext(context.Background(), dir)
}

// ScanFilesContext is ScanFiles with cancellation support during directory
// traversal and content hashing.
func (m *Manifest) ScanFilesContext(ctx context.Context, dir string) error {
	var entries []FileEntry
	var totalSize int64
	hasher := sha256.New()

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		entry := FileEntry{
			Path: rel,
			Size: info.Size(),
		}
		// Symlinks: hash the link target text instead of dereferencing it
		// (os.Open follows links and would fail on a dangling target). Keeps the
		// link in the manifest and content hash, and is deterministic so a
		// restored tree re-hashes identically.
		if info.Mode()&os.ModeSymlink != 0 {
			linkDest, lerr := os.Readlink(path)
			if lerr != nil {
				return nil // skip links whose metadata can't be read
			}
			sum := sha256.Sum256([]byte("symlink:" + linkDest))
			entry.Hash = hex.EncodeToString(sum[:])
			entry.Size = int64(len(linkDest))
			hasher.Write([]byte(entry.Path))
			hasher.Write([]byte(entry.Hash))
			entries = append(entries, entry)
			totalSize += entry.Size
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type at %q: %s", path, info.Mode().Type())
		}
		// Hash file content for dedup and integrity
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		defer f.Close() //nolint:errcheck
		fileHash := sha256.New()
		if _, err := copyHash(fileHash, &contextFileReader{ctx: ctx, reader: f}); err != nil {
			return fmt.Errorf("hash %s: %w", path, err)
		}
		entry.Hash = hex.EncodeToString(fileHash.Sum(nil))
		// Add to overall content hash
		hasher.Write([]byte(entry.Path))
		hasher.Write([]byte(entry.Hash))
		entries = append(entries, entry)
		totalSize += info.Size()
		return nil
	})
	if err != nil {
		return err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	m.Files = entries
	m.FileCount = len(entries)
	m.TotalSize = totalSize
	m.ContentHash = hex.EncodeToString(hasher.Sum(nil))
	return nil
}

// Save writes the manifest to manifest.json in the given directory. The write is
// atomic (temp file + rename) so an interrupted write never leaves a truncated
// or half-written manifest.json — which, since the manifest is what makes a stash
// discoverable, would otherwise hide or corrupt an otherwise-intact stash.
func (m *Manifest) Save(dir string) error {
	path := filepath.Join(dir, "manifest.json")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".manifest-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp manifest: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp manifest: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sync temp manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp manifest: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename manifest into place: %w", err)
	}
	return nil
}

// Load reads a manifest from manifest.json in the given directory.
func Load(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "manifest.json")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("lstat manifest: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("manifest is not a regular file")
	}
	if info.Size() > maxManifestBytes {
		return nil, fmt.Errorf("manifest exceeds %d-byte size limit", maxManifestBytes)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer f.Close() //nolint:errcheck
	data, err := io.ReadAll(io.LimitReader(f, maxManifestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if int64(len(data)) > maxManifestBytes {
		return nil, fmt.Errorf("manifest exceeds %d-byte size limit", maxManifestBytes)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
	}
	// Early manifests did not always persist schema_version. They have the same
	// shape as 1.0, so adopt them explicitly; reject unknown future schemas
	// instead of silently interpreting fields with outdated semantics.
	if m.SchemaVersion == "" {
		m.SchemaVersion = SchemaVersion
	}
	if m.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported manifest schema %q (supported: %s)", m.SchemaVersion, SchemaVersion)
	}
	if m.ID == "" {
		return nil, errors.New("manifest id is required")
	}
	if m.FileCount < 0 || m.TotalSize < 0 || m.CompressedSize < 0 {
		return nil, errors.New("manifest counts and sizes must not be negative")
	}
	if _, err := time.Parse(time.RFC3339, m.CreatedAt); err != nil {
		return nil, fmt.Errorf("invalid manifest created_at %q: %w", m.CreatedAt, err)
	}
	if m.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, m.ExpiresAt); err != nil {
			return nil, fmt.Errorf("invalid manifest expires_at %q: %w", m.ExpiresAt, err)
		}
	}
	seenPaths := make(map[string]struct{}, len(m.Files))
	var filesSize int64
	for i, file := range m.Files {
		clean := filepath.Clean(file.Path)
		if file.Path == "" || filepath.IsAbs(file.Path) || clean == "." || clean == ".." ||
			strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean != file.Path {
			return nil, fmt.Errorf("invalid manifest file path at index %d: %q", i, file.Path)
		}
		if file.Size < 0 {
			return nil, fmt.Errorf("invalid negative size for manifest file %q", file.Path)
		}
		if _, exists := seenPaths[file.Path]; exists {
			return nil, fmt.Errorf("duplicate manifest file path %q", file.Path)
		}
		if file.Hash != "" {
			if decoded, err := hex.DecodeString(file.Hash); err != nil || len(decoded) != sha256.Size {
				return nil, fmt.Errorf("invalid SHA-256 hash for manifest file %q", file.Path)
			}
		}
		seenPaths[file.Path] = struct{}{}
		if file.Size > math.MaxInt64-filesSize {
			return nil, errors.New("manifest file sizes overflow int64")
		}
		filesSize += file.Size
	}
	if m.Files != nil {
		if m.FileCount != len(m.Files) {
			return nil, fmt.Errorf("manifest file_count %d does not match %d file entries", m.FileCount, len(m.Files))
		}
		if m.TotalSize != filesSize {
			return nil, fmt.Errorf("manifest total_size %d does not match file entries totaling %d", m.TotalSize, filesSize)
		}
	}
	if m.ContentHash != "" {
		if decoded, err := hex.DecodeString(m.ContentHash); err != nil || len(decoded) != sha256.Size {
			return nil, fmt.Errorf("invalid manifest content_hash")
		}
	}
	return &m, nil
}

// VideoSummary returns a human-readable source-video summary for vidtrace
// bundles, e.g. "/Downloads/bug.mp4 (120s, 30fps)", or "" if not applicable.
func (m *Manifest) VideoSummary() string {
	v := m.Custom["source_video"]
	if v == "" {
		return ""
	}
	extra := ""
	if d := m.Custom["duration_seconds"]; d != "" {
		extra = d + "s"
	}
	if fr := m.Custom["frame_rate"]; fr != "" {
		if extra != "" {
			extra += ", "
		}
		extra += fr + "fps"
	}
	if extra != "" {
		v += " (" + extra + ")"
	}
	return v
}

// HasTag returns true if the manifest contains the given tag.
func (m *Manifest) HasTag(tag string) bool {
	for _, t := range m.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// AddTag adds a tag if not already present.
func (m *Manifest) AddTag(tag string) {
	if !m.HasTag(tag) {
		m.Tags = append(m.Tags, tag)
	}
}

// SearchableText returns a concatenation of file paths for indexing.
func (m *Manifest) SearchableText() string {
	var buf []byte
	for _, f := range m.Files {
		buf = append(buf, []byte(f.Path)...)
		buf = append(buf, '\n')
	}
	return string(buf)
}

func copyHash(dst hashWriter, src fileReader) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			dst.Write(buf[:n]) //nolint:errcheck
			total += int64(n)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}
			return total, err
		}
	}
}

type hashWriter interface {
	Write(p []byte) (n int, err error)
	Sum(b []byte) []byte
}

type fileReader interface {
	Read(p []byte) (n int, err error)
}

type contextFileReader struct {
	ctx    context.Context
	reader fileReader
}

func (r *contextFileReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
