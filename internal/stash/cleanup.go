package stash

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
)

// CleanupCategory represents a reason a stash might be cleanup-eligible.
type CleanupCategory string

const (
	CatExpired    CleanupCategory = "expired"
	CatOrphaned   CleanupCategory = "orphaned"    // source path no longer exists
	CatSuperseded CleanupCategory = "superseded"  // newer stash for same tool+source_path exists
	CatDuplicate  CleanupCategory = "duplicate"   // same content_hash as another stash
	CatBranchGone CleanupCategory = "branch-gone" // branch tag references deleted git branch
	CatStale      CleanupCategory = "stale"       // not accessed/restored in N days
	CatKeep       CleanupCategory = "keep"        // no cleanup reason found
)

// CleanupRecommendation is the analysis result for a single stash.
type CleanupRecommendation struct {
	ID       string          `json:"id"`
	Name     string          `json:"name,omitempty"`
	Tool     string          `json:"tool,omitempty"`
	Category CleanupCategory `json:"category"`
	Reason   string          `json:"reason"`
	Size     int64           `json:"size"`
}

// CleanupResult is the full analysis output.
type CleanupResult struct {
	Total           int                     `json:"total"`
	Recommendations []CleanupRecommendation `json:"recommendations"`
	ByCategory      map[CleanupCategory]int `json:"by_category"`
	Reclaimable     int64                   `json:"reclaimable"`
}

// CleanupOptions controls the analysis.
type CleanupOptions struct {
	StaleDays   int      // days without access to be considered stale (0 = disable)
	ProjectsDir string   // path to ~/projects for orphan detection (optional)
	NotesDir    string   // path to ~/notes/projects for orphan detection (optional)
	Categories  []string // filter to specific categories (empty = all)
}

// categoryPriority defines the order in which categories are checked.
// The first matching category wins — a stash is never double-counted.
var categoryPriority = []CleanupCategory{
	CatExpired,
	CatOrphaned,
	CatSuperseded,
	CatDuplicate,
	CatBranchGone,
	CatStale,
	CatKeep,
}

// AnalyzeCleanup scans all stashes and returns cleanup recommendations.
func (m *Manager) AnalyzeCleanup(ctx context.Context, opts CleanupOptions) (*CleanupResult, error) {
	// 1. List all stashes including expired ones.
	stashes, err := m.ListFiltered(ctx, ListOptions{IncludeExpired: true})
	if err != nil {
		return nil, fmt.Errorf("list stashes for cleanup analysis: %w", err)
	}

	// 2. Build lookup maps for supersession and duplicate detection.

	// supersededKey -> stashes sorted by created_at descending.
	// All but the first in each group are superseded.
	supersededIndex := make(map[string][]*Stash)
	for _, st := range stashes {
		key := supersededKey(st.Manifest)
		if key == "" {
			continue
		}
		supersededIndex[key] = append(supersededIndex[key], st)
	}
	for key := range supersededIndex {
		group := supersededIndex[key]
		sort.Slice(group, func(i, j int) bool {
			return group[i].Manifest.CreatedAt > group[j].Manifest.CreatedAt
		})
		supersededIndex[key] = group
	}

	// content_hash -> stashes sorted by created_at descending.
	// All but the first are duplicates.
	duplicateIndex := make(map[string][]*Stash)
	for _, st := range stashes {
		if st.Manifest.ContentHash == "" {
			continue
		}
		duplicateIndex[st.Manifest.ContentHash] = append(duplicateIndex[st.Manifest.ContentHash], st)
	}
	for hash := range duplicateIndex {
		group := duplicateIndex[hash]
		sort.Slice(group, func(i, j int) bool {
			return group[i].Manifest.CreatedAt > group[j].Manifest.CreatedAt
		})
		duplicateIndex[hash] = group
	}

	// Build sets of superseded and duplicate IDs (all but the newest in each group).
	supersededIDs := make(map[string]bool)
	for _, group := range supersededIndex {
		for i := 1; i < len(group); i++ {
			supersededIDs[group[i].Manifest.ID] = true
		}
	}
	duplicateIDs := make(map[string]bool)
	for _, group := range duplicateIndex {
		for i := 1; i < len(group); i++ {
			duplicateIDs[group[i].Manifest.ID] = true
		}
	}

	// 3. Category filter set (for opts.Categories filtering).
	var catFilter map[string]bool
	if len(opts.Categories) > 0 {
		catFilter = make(map[string]bool, len(opts.Categories))
		for _, c := range opts.Categories {
			catFilter[strings.TrimSpace(c)] = true
		}
	}

	// 4. Analyze each stash.
	res := &CleanupResult{
		Total:           len(stashes),
		ByCategory:      make(map[CleanupCategory]int),
		Recommendations: make([]CleanupRecommendation, 0, len(stashes)),
	}

	for _, st := range stashes {
		rec := m.classifyStash(st, opts, supersededIDs, duplicateIDs)

		// Apply category filter: skip categories the caller didn't ask for.
		// "keep" is always included unless the caller explicitly filters it out.
		if catFilter != nil && !catFilter[string(rec.Category)] {
			continue
		}

		res.Recommendations = append(res.Recommendations, rec)
		res.ByCategory[rec.Category]++

		// Reclaimable size: only non-keep categories contribute.
		if rec.Category != CatKeep {
			res.Reclaimable += rec.Size
		}
	}

	// Sort recommendations by priority order, then by size descending within
	// each category.
	sort.SliceStable(res.Recommendations, func(i, j int) bool {
		catI := categoryOrder(res.Recommendations[i].Category)
		catJ := categoryOrder(res.Recommendations[j].Category)
		if catI != catJ {
			return catI < catJ
		}
		return res.Recommendations[i].Size > res.Recommendations[j].Size
	})

	return res, nil
}

// classifyStash determines the cleanup category for a single stash by checking
// each category in priority order and returning the first match.
func (m *Manager) classifyStash(st *Stash, opts CleanupOptions, supersededIDs, duplicateIDs map[string]bool) CleanupRecommendation {
	man := st.Manifest
	rec := CleanupRecommendation{
		ID:   man.ID,
		Name: man.Name,
		Tool: man.Tool,
		Size: man.TotalSize,
	}

	// Check categories in priority order.
	for _, cat := range categoryPriority {
		reason, match := m.checkCategory(st, man, cat, opts, supersededIDs, duplicateIDs)
		if match {
			rec.Category = cat
			rec.Reason = reason
			return rec
		}
	}

	// No category matched — keep.
	rec.Category = CatKeep
	rec.Reason = "no cleanup reason found"
	return rec
}

// checkCategory tests whether a stash matches a single category. Returns the
// human-readable reason and whether the category matched.
func (m *Manager) checkCategory(st *Stash, man *manifest.Manifest, cat CleanupCategory, opts CleanupOptions, supersededIDs, duplicateIDs map[string]bool) (string, bool) {
	switch cat {
	case CatExpired:
		if IsExpired(man) {
			return fmt.Sprintf("TTL expired (%s)", man.ExpiresAt), true
		}

	case CatOrphaned:
		if reason, ok := m.checkOrphaned(man, opts); ok {
			return reason, true
		}

	case CatSuperseded:
		if supersededIDs[man.ID] {
			key := supersededKey(man)
			return fmt.Sprintf("newer stash exists for same tool+source (%s)", key), true
		}

	case CatDuplicate:
		if duplicateIDs[man.ID] {
			return fmt.Sprintf("duplicate content hash (newer copy exists: %s)", man.ContentHash), true
		}

	case CatBranchGone:
		if reason, ok := checkBranchGone(man); ok {
			return reason, true
		}

	case CatStale:
		if opts.StaleDays > 0 {
			if reason, ok := checkStale(man, opts.StaleDays); ok {
				return reason, true
			}
		}

	case CatKeep:
		// "keep" is the fallback — always matches as the last priority.
		return "no cleanup reason found", true
	}

	return "", false
}

// checkOrphaned checks if the stash's source path no longer exists. When
// opts.ProjectsDir is set, it also checks if the project name extracted from
// the source path exists under ProjectsDir.
func (m *Manager) checkOrphaned(man *manifest.Manifest, opts CleanupOptions) (string, bool) {
	if man.SourcePath == "" {
		return "", false
	}

	// Direct check: does the source path still exist?
	if _, err := os.Stat(man.SourcePath); os.IsNotExist(err) {
		return fmt.Sprintf("source path no longer exists (%s)", man.SourcePath), true
	}

	// If ProjectsDir is set, check if the project directory still exists
	// under it. The project name is the base of the source path.
	if opts.ProjectsDir != "" {
		projectName := extractProjectName(man.SourcePath)
		if projectName != "" {
			projectPath := filepath.Join(opts.ProjectsDir, projectName)
			if _, err := os.Stat(projectPath); os.IsNotExist(err) {
				return fmt.Sprintf("project directory gone (%s)", projectPath), true
			}
		}
	}

	// If NotesDir is set, check if the project directory exists under it.
	if opts.NotesDir != "" {
		projectName := extractProjectName(man.SourcePath)
		if projectName != "" {
			notesPath := filepath.Join(opts.NotesDir, projectName)
			if _, err := os.Stat(notesPath); os.IsNotExist(err) {
				return fmt.Sprintf("project directory gone (%s)", notesPath), true
			}
		}
	}

	return "", false
}

// checkBranchGone checks if the stash has a "branch:" tag referencing a git
// branch that no longer exists in the source path's repository. If git fails
// or the source path doesn't exist, this check is skipped (returns false).
func checkBranchGone(man *manifest.Manifest) (string, bool) {
	var branchTags []string
	for _, tag := range man.Tags {
		if strings.HasPrefix(tag, "branch:") {
			branchTags = append(branchTags, tag)
		}
	}
	if len(branchTags) == 0 {
		return "", false
	}

	// We need a source path to run git against.
	if man.SourcePath == "" {
		return "", false
	}
	if _, err := os.Stat(man.SourcePath); err != nil {
		// Source path doesn't exist — can't run git. Skip this check.
		return "", false
	}

	for _, tag := range branchTags {
		branch := strings.TrimPrefix(tag, "branch:")
		branch = strings.TrimSpace(branch)
		if branch == "" {
			continue
		}

		// Run: git -C <source_path> branch --list <branch>
		cmd := exec.Command("git", "-C", man.SourcePath, "branch", "--list", branch)
		output, err := cmd.Output()
		if err != nil {
			// git failed (not a repo, git not installed, etc.) — skip.
			continue
		}
		if len(strings.TrimSpace(string(output))) == 0 {
			return fmt.Sprintf("branch no longer exists in git (%s)", branch), true
		}
	}

	return "", false
}

// checkStale checks if the stash's created_at is older than staleDays. Since
// we don't track last_accessed yet, created_at is used as a proxy.
func checkStale(man *manifest.Manifest, staleDays int) (string, bool) {
	if man.CreatedAt == "" {
		return "", false
	}
	created, err := time.Parse(time.RFC3339, man.CreatedAt)
	if err != nil {
		return "", false
	}
	age := time.Since(created)
	staleThreshold := time.Duration(staleDays) * 24 * time.Hour
	if age > staleThreshold {
		days := int(age.Hours() / 24)
		return fmt.Sprintf("stale: %d days since creation (threshold: %d)", days, staleDays), true
	}
	return "", false
}

// supersededKey returns the dedup key for supersession detection:
// "tool|source_path". Returns "" if either is empty.
func supersededKey(man *manifest.Manifest) string {
	if man.Tool == "" || man.SourcePath == "" {
		return ""
	}
	return man.Tool + "|" + man.SourcePath
}

// extractProjectName extracts the project directory name from a source path.
// For "/Users/foo/projects/myapp/src", it returns "myapp" (the segment after
// "projects"). If no "projects" segment is found, it returns the parent of the
// base if base is a common leaf (src, cmd, internal, pkg), otherwise the base
// name itself. If the path doesn't contain a recognizable structure, it
// returns the base directory name.
func extractProjectName(sourcePath string) string {
	if sourcePath == "" {
		return ""
	}
	clean := filepath.Clean(sourcePath)
	parts := strings.Split(clean, string(filepath.Separator))

	// Look for a "projects" segment and return the next element.
	for i, p := range parts {
		if p == "projects" && i+1 < len(parts) && parts[i+1] != "" {
			return parts[i+1]
		}
	}

	// Fallback: if base is a common leaf dir (src, cmd, internal, pkg),
	// return the parent directory name instead.
	base := filepath.Base(clean)
	commonLeafs := map[string]bool{
		"src":      true,
		"cmd":      true,
		"internal": true,
		"pkg":      true,
		"dist":     true,
		"build":    true,
	}
	if commonLeafs[base] {
		parent := filepath.Dir(clean)
		name := filepath.Base(parent)
		if name != "." && name != string(filepath.Separator) {
			return name
		}
		return ""
	}

	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}

// categoryOrder returns the numeric priority of a category (lower = higher
// priority). Used for sorting.
func categoryOrder(cat CleanupCategory) int {
	for i, c := range categoryPriority {
		if c == cat {
			return i
		}
	}
	return len(categoryPriority)
}

// CategoryDisplay returns a human-readable name for a cleanup category,
// suitable for CLI/table display.
func CategoryDisplay(cat CleanupCategory) string {
	switch cat {
	case CatExpired:
		return "Expired"
	case CatOrphaned:
		return "Orphaned"
	case CatSuperseded:
		return "Superseded"
	case CatDuplicate:
		return "Duplicate"
	case CatBranchGone:
		return "Branch Gone"
	case CatStale:
		return "Stale"
	case CatKeep:
		return "Keep"
	default:
		return string(cat)
	}
}
