// Package diff compares stash content against a live directory.
package diff

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
)

// DiffResult holds the comparison result.
type DiffResult struct {
	OnlyInStash []string
	OnlyInTarget []string
	Changed      []ChangedFile
	Unchanged    int
}

// ChangedFile describes a file that exists in both but differs.
type ChangedFile struct {
	Path      string
	StashSize int64
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

	// Walk target directory
	targetFiles := make(map[string]int64)
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
		targetFiles[rel] = info.Size()
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

	// Changed files (exist in both but different size)
	for path, stashEntry := range stashFiles {
		if targetSize, exists := targetFiles[path]; exists {
			if stashEntry.Size != targetSize {
				result.Changed = append(result.Changed, ChangedFile{
					Path:       path,
					StashSize:  stashEntry.Size,
					TargetSize: targetSize,
				})
			} else {
				result.Unchanged++
			}
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
			sb.WriteString(fmt.Sprintf("  + %s\n", f))
		}
	}
	if len(r.OnlyInTarget) > 0 {
		sb.WriteString("Only in target:\n")
		for _, f := range r.OnlyInTarget {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}
	if len(r.Changed) > 0 {
		sb.WriteString("Changed (size differs):\n")
		for _, f := range r.Changed {
			sb.WriteString(fmt.Sprintf("  ~ %s (stash: %d bytes, target: %d bytes)\n", f.Path, f.StashSize, f.TargetSize))
		}
	}
	if r.Unchanged > 0 {
		sb.WriteString(fmt.Sprintf("Unchanged: %d files\n", r.Unchanged))
	}
	if sb.Len() == 0 {
		sb.WriteString("No differences found.\n")
	}
	return sb.String()
}