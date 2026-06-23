// Package manifest defines the metadata structure for a stash snapshot.
package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	var entries []FileEntry
	var totalSize int64
	hasher := sha256.New()

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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
		// Hash file content for dedup and integrity
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", path, err)
		}
		defer f.Close() //nolint:errcheck
		fileHash := sha256.New()
		if _, err := copyHash(fileHash, f); err != nil {
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

// Save writes the manifest to manifest.json in the given directory.
func (m *Manifest) Save(dir string) error {
	path := filepath.Join(dir, "manifest.json")
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// Load reads a manifest from manifest.json in the given directory.
func Load(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal manifest: %w", err)
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
			if err.Error() == "EOF" {
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
