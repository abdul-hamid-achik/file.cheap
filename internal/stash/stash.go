// Package stash implements the core stash operations: Save, Restore, Drop, List, Info.
package stash

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/apperror"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
)

// Options for saving a stash.
type SaveOptions struct {
	SourcePath string   // path to the source file or directory
	Name       string   // optional display name
	Tags       []string  // tags for categorization
	Tool       string   // tool that produced the content (e.g., "vidtrace")
	Custom     map[string]string // agent-provided metadata
}

// Stash represents a saved snapshot with its manifest.
type Stash struct {
	Manifest *manifest.Manifest
	Dir      string // path to the stash directory on disk
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

// StashDir returns the directory for a given stash ID.
func (m *Manager) StashDir(id string) string {
	return filepath.Join(m.rootDir, id)
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

	// Detect bundle type
	man.BundleType = detectBundleType(contentDir)

	if err := man.Save(stashDir); err != nil {
		_ = os.RemoveAll(stashDir)
		return nil, fmt.Errorf("save manifest: %w", err)
	}

	return &Stash{Manifest: man, Dir: stashDir}, nil
}

// Restore extracts a stash to the given target directory.
// If target is empty, uses a temp directory.
func (m *Manager) Restore(ctx context.Context, id, target string) error {
	stashDir := m.StashDir(id)
	if _, err := os.Stat(stashDir); err != nil {
		if os.IsNotExist(err) {
			return apperror.ErrStashNotFound
		}
		return fmt.Errorf("stat stash: %w", err)
	}

	// Check for compressed archive
	archivePath := filepath.Join(stashDir, "content.tar.zst")
	contentDir := filepath.Join(stashDir, "content")
	hasArchive := fileExists(archivePath)
	hasExtracted := dirExists(contentDir)

	if target == "" {
		target = filepath.Join(os.TempDir(), id)
	}

	if err := os.MkdirAll(target, 0755); err != nil {
		return fmt.Errorf("create target dir: %w", err)
	}

	if hasExtracted {
		if err := copyDir(contentDir, target); err != nil {
			return apperror.WrapWithMessage(err, "restore_failed", "failed to copy content")
		}
	} else if hasArchive {
		if err := extractArchive(archivePath, target); err != nil {
			return apperror.WrapWithMessage(err, "restore_failed", "failed to extract archive")
		}
	} else {
		return apperror.New("restore_failed", "no content or archive found in stash")
	}

	return nil
}

// Drop removes a stash entirely.
func (m *Manager) Drop(ctx context.Context, id string) error {
	stashDir := m.StashDir(id)
	if _, err := os.Stat(stashDir); err != nil {
		if os.IsNotExist(err) {
			return apperror.ErrStashNotFound
		}
		return fmt.Errorf("stat stash: %w", err)
	}
	return os.RemoveAll(stashDir)
}

// List returns all stashes, optionally filtered by tag.
func (m *Manager) List(ctx context.Context, tag string) ([]*Stash, error) {
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
		if tag != "" && !man.HasTag(tag) {
			continue
		}
		stashes = append(stashes, &Stash{Manifest: man, Dir: stashDir})
	}
	return stashes, nil
}

// Info returns detailed info about a single stash.
func (m *Manager) Info(ctx context.Context, id string) (*Stash, error) {
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

// Exists returns true if a stash with the given ID exists.
func (m *Manager) Exists(id string) bool {
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

// detectBundleType checks for known bundle structures.
func detectBundleType(contentDir string) string {
	// vidtrace bundle: has metadata.json + timeline.json
	if fileExists(filepath.Join(contentDir, "metadata.json")) &&
		fileExists(filepath.Join(contentDir, "timeline.json")) {
		return "vidtrace"
	}
	return "generic"
}

// extractArchive is a placeholder — the compress package handles actual extraction.
func extractArchive(archivePath, target string) error {
	// This is wired up by the compress package via SetExtractor
	if defaultExtractor != nil {
		return defaultExtractor(archivePath, target)
	}
	return fmt.Errorf("no archive extractor configured")
}

// Default extractor set by the compress package
var defaultExtractor ExtractFunc

// ExtractFunc extracts an archive to a target directory.
type ExtractFunc func(archivePath, target string) error

// SetExtractor registers the archive extractor (called by compress package init).
func SetExtractor(fn ExtractFunc) {
	defaultExtractor = fn
}