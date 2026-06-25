// Package diff compares stash content against a live directory.
package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
)

// DiffResult holds the comparison result.
type DiffResult struct {
	OnlyInStash  []string
	OnlyInTarget []string
	Changed      []ChangedFile
	Unchanged    int
}

// ChangedFile describes a file that exists in both but differs.
type ChangedFile struct {
	Path       string
	StashSize  int64
	TargetSize int64
}

// CompareStashToDir compares the content of a stash against a target directory.
func CompareStashToDir(stashDir, targetDir string) (*DiffResult, error) {
	man, err := manifest.Load(stashDir)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}

	// Build set of stash files
	stashFiles := make(map[string]manifest.FileEntry)
	for _, f := range man.Files {
		stashFiles[f.Path] = f
	}

	// Walk target directory, recording size + content hash for each file.
	type targetFile struct {
		size int64
		hash string
	}
	targetFiles := make(map[string]targetFile)
	err = filepath.Walk(targetDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(targetDir, path)
		if err != nil {
			return err
		}
		h, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("hash %s: %w", rel, err)
		}
		targetFiles[rel] = targetFile{size: info.Size(), hash: h}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk target: %w", err)
	}

	result := &DiffResult{}

	// Files only in stash
	for path := range stashFiles {
		if _, exists := targetFiles[path]; !exists {
			result.OnlyInStash = append(result.OnlyInStash, path)
		}
	}

	// Files only in target
	for path := range targetFiles {
		if _, exists := stashFiles[path]; !exists {
			result.OnlyInTarget = append(result.OnlyInTarget, path)
		}
	}

	// Changed files: exist in both but differ by content hash. Fall back to a
	// size comparison only when the stash manifest predates content hashing.
	for path, stashEntry := range stashFiles {
		tf, exists := targetFiles[path]
		if !exists {
			continue
		}
		var changed bool
		if stashEntry.Hash != "" && tf.hash != "" {
			changed = stashEntry.Hash != tf.hash
		} else {
			changed = stashEntry.Size != tf.size
		}
		if changed {
			result.Changed = append(result.Changed, ChangedFile{
				Path:       path,
				StashSize:  stashEntry.Size,
				TargetSize: tf.size,
			})
		} else {
			result.Unchanged++
		}
	}

	sort.Strings(result.OnlyInStash)
	sort.Strings(result.OnlyInTarget)
	sort.Slice(result.Changed, func(i, j int) bool {
		return result.Changed[i].Path < result.Changed[j].Path
	})

	return result, nil
}

// Format returns a human-readable diff report.
func (r *DiffResult) Format() string {
	var sb strings.Builder
	if len(r.OnlyInStash) > 0 {
		sb.WriteString("Only in stash:\n")
		for _, f := range r.OnlyInStash {
			fmt.Fprintf(&sb, "  + %s\n", f)
		}
	}
	if len(r.OnlyInTarget) > 0 {
		sb.WriteString("Only in target:\n")
		for _, f := range r.OnlyInTarget {
			fmt.Fprintf(&sb, "  - %s\n", f)
		}
	}
	if len(r.Changed) > 0 {
		sb.WriteString("Changed (content differs):\n")
		for _, f := range r.Changed {
			fmt.Fprintf(&sb, "  ~ %s (stash: %d bytes, target: %d bytes)\n", f.Path, f.StashSize, f.TargetSize)
		}
	}
	if r.Unchanged > 0 {
		fmt.Fprintf(&sb, "Unchanged: %d files\n", r.Unchanged)
	}
	if sb.Len() == 0 {
		sb.WriteString("No differences found.\n")
	}
	return sb.String()
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
