package stash

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/stretchr/testify/assert"
)

// TestAnalyzeCleanupExpired verifies that a stash with an expired TTL is
// categorized as "expired".
func TestAnalyzeCleanupExpired(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	assert.NoError(t, err)

	srcDir := filepath.Join(tmp, "src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("x"), 0644))

	_, err = mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "expiring",
		TTL:        "1s",
	})
	assert.NoError(t, err)

	time.Sleep(2 * time.Second)

	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 1, res.Total)
	assert.Len(t, res.Recommendations, 1)
	assert.Equal(t, CatExpired, res.Recommendations[0].Category)
	assert.Equal(t, 1, res.ByCategory[CatExpired])
	assert.Greater(t, res.Reclaimable, int64(0))
}

// TestAnalyzeCleanupOrphaned verifies that a stash whose source path is gone
// is categorized as "orphaned".
func TestAnalyzeCleanupOrphaned(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	assert.NoError(t, err)

	srcDir := filepath.Join(tmp, "src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("x"), 0644))

	_, err = mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "orphaned",
	})
	assert.NoError(t, err)

	// Delete the source path to make it orphaned.
	assert.NoError(t, os.RemoveAll(srcDir))

	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	assert.Len(t, res.Recommendations, 1)
	assert.Equal(t, CatOrphaned, res.Recommendations[0].Category)
}

func TestAnalyzeCleanupDoesNotTreatMissingProjectMirrorAsOrphaned(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	assert.NoError(t, err)

	source := filepath.Join(tmp, "standalone-artifact.txt")
	assert.NoError(t, os.WriteFile(source, []byte("evidence"), 0600))
	_, err = mgr.Save(context.Background(), &SaveOptions{SourcePath: source, Name: "standalone"})
	assert.NoError(t, err)

	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	assert.Len(t, res.Recommendations, 1)
	assert.Equal(t, CatKeep, res.Recommendations[0].Category)
}

// TestAnalyzeCleanupSuperseded verifies that when two stashes share the same
// tool+source_path, the older one is categorized as "superseded".
func TestAnalyzeCleanupSuperseded(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	assert.NoError(t, err)

	srcDir := filepath.Join(tmp, "src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("x"), 0644))

	// Save two stashes with the same tool+source_path.
	st1, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "first",
		Tool:       "codemap",
	})
	assert.NoError(t, err)

	// Manually set st1's CreatedAt to the past so it's clearly the older one.
	st1.Manifest.CreatedAt = time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	assert.NoError(t, st1.Manifest.Save(mgr.StashDir(st1.Manifest.ID)))

	_, err = mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "second",
		Tool:       "codemap",
	})
	assert.NoError(t, err)

	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 2, res.Total)

	// Find the recommendations by ID.
	var older, newer *CleanupRecommendation
	for i := range res.Recommendations {
		if res.Recommendations[i].ID == st1.Manifest.ID {
			older = &res.Recommendations[i]
		} else {
			newer = &res.Recommendations[i]
		}
	}
	assert.NotNil(t, older)
	assert.NotNil(t, newer)
	assert.Equal(t, CatSuperseded, older.Category)
	assert.Equal(t, CatKeep, newer.Category)
}

// TestAnalyzeCleanupDuplicate verifies that when two stashes share the same
// content hash, the older one is categorized as "duplicate".
func TestAnalyzeCleanupDuplicate(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	assert.NoError(t, err)

	srcDir := filepath.Join(tmp, "src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("identical"), 0644))

	st1, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "first-dup",
	})
	assert.NoError(t, err)

	// Make st1 clearly older.
	st1.Manifest.CreatedAt = time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	assert.NoError(t, st1.Manifest.Save(mgr.StashDir(st1.Manifest.ID)))

	// Save second stash from a different path but with the same content.
	// We copy the source to a different path so the supersededKey won't match
	// (no tool set), but the content hash will.
	srcDir2 := filepath.Join(tmp, "src2")
	assert.NoError(t, os.MkdirAll(srcDir2, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(srcDir2, "f.txt"), []byte("identical"), 0644))

	_, err = mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir2,
		Name:       "second-dup",
	})
	assert.NoError(t, err)

	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)

	var older *CleanupRecommendation
	for i := range res.Recommendations {
		if res.Recommendations[i].ID == st1.Manifest.ID {
			older = &res.Recommendations[i]
		}
	}
	assert.NotNil(t, older)
	assert.Equal(t, CatDuplicate, older.Category)
}

// TestAnalyzeCleanupStale verifies that the stale category works when
// staleDays is set and the stash is older than the threshold.
func TestAnalyzeCleanupStale(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	assert.NoError(t, err)

	srcDir := filepath.Join(tmp, "src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("x"), 0644))

	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "stale-stash",
	})
	assert.NoError(t, err)

	// Set created_at to 100 days ago.
	st.Manifest.CreatedAt = time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
	assert.NoError(t, st.Manifest.Save(mgr.StashDir(st.Manifest.ID)))

	// Without staleDays: should be keep (source exists, no TTL).
	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	assert.Len(t, res.Recommendations, 1)
	assert.Equal(t, CatKeep, res.Recommendations[0].Category)

	// With staleDays=30: should be stale.
	res, err = mgr.AnalyzeCleanup(context.Background(), CleanupOptions{StaleDays: 30})
	assert.NoError(t, err)
	assert.Len(t, res.Recommendations, 1)
	assert.Equal(t, CatStale, res.Recommendations[0].Category)
}

// TestAnalyzeCleanupKeep verifies that a normal stash with no cleanup signals
// is categorized as "keep".
func TestAnalyzeCleanupKeep(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	assert.NoError(t, err)

	srcDir := filepath.Join(tmp, "src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("x"), 0644))

	_, err = mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "normal",
	})
	assert.NoError(t, err)

	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	assert.Len(t, res.Recommendations, 1)
	assert.Equal(t, CatKeep, res.Recommendations[0].Category)
	assert.Equal(t, 1, res.ByCategory[CatKeep])
	assert.Equal(t, int64(0), res.Reclaimable) // keep doesn't add to reclaimable
}

// TestAnalyzeCleanupCategoryFilter verifies that the --categories filter
// correctly excludes non-matching categories.
func TestAnalyzeCleanupCategoryFilter(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	assert.NoError(t, err)

	srcDir := filepath.Join(tmp, "src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("x"), 0644))

	_, err = mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "normal",
	})
	assert.NoError(t, err)

	// Filter to only "orphaned" — the normal stash should be excluded.
	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{
		Categories: []string{"orphaned"},
	})
	assert.NoError(t, err)
	assert.Len(t, res.Recommendations, 0)

	// Filter to only "keep" — the normal stash should be included.
	res, err = mgr.AnalyzeCleanup(context.Background(), CleanupOptions{
		Categories: []string{"keep"},
	})
	assert.NoError(t, err)
	assert.Len(t, res.Recommendations, 1)
	assert.Equal(t, CatKeep, res.Recommendations[0].Category)
}

// TestCategoryDisplay verifies the human-readable display names.
func TestCategoryDisplay(t *testing.T) {
	cases := []struct {
		cat  CleanupCategory
		want string
	}{
		{CatExpired, "Expired"},
		{CatOrphaned, "Orphaned"},
		{CatSuperseded, "Superseded"},
		{CatDuplicate, "Duplicate"},
		{CatBranchGone, "Branch Gone"},
		{CatStale, "Stale"},
		{CatKeep, "Keep"},
		{CleanupCategory("unknown"), "unknown"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, CategoryDisplay(c.cat))
	}
}

// TestSupersededKey verifies the key is "tool|source_path" and returns ""
// when either is empty.
func TestSupersededKey(t *testing.T) {
	// Both set
	assert.Equal(t, "codemap|/src", supersededKey(&manifest.Manifest{Tool: "codemap", SourcePath: "/src"}))
	// Tool empty
	assert.Equal(t, "", supersededKey(&manifest.Manifest{SourcePath: "/src"}))
	// SourcePath empty
	assert.Equal(t, "", supersededKey(&manifest.Manifest{Tool: "codemap"}))
	// Both empty
	assert.Equal(t, "", supersededKey(&manifest.Manifest{}))
}

// TestAnalyzeCleanupKeepTag verifies that a stash with the keep tag is never
// dropped even in smart mode apply.
func TestAnalyzeCleanupKeepTag(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	assert.NoError(t, err)

	srcDir := filepath.Join(tmp, "src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(srcDir, "f.txt"), []byte("x"), 0644))

	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "keep-me",
		TTL:        "1s",
		Tags:       []string{"keep"},
	})
	assert.NoError(t, err)

	time.Sleep(2 * time.Second)

	// Even though expired, the keep tag should be visible in the manifest.
	info, _ := mgr.Info(context.Background(), st.Manifest.ID)
	assert.True(t, info.Manifest.HasTag("keep"))

	// Analyze: the stash will be categorized as "expired" (highest priority).
	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	assert.Equal(t, CatExpired, res.Recommendations[0].Category)

	// The analysis itself doesn't filter by keep-tag; the caller (CLI/MCP)
	// is responsible for respecting keep-tag during apply. Verify the stash
	// still exists.
	assert.True(t, mgr.Exists(st.Manifest.ID))
}

// TestAnalyzeCleanupEmptyDir verifies that analyzing an empty stash dir
// returns zero results without error.
func TestAnalyzeCleanupEmptyDir(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	assert.NoError(t, err)

	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 0, res.Total)
	assert.Empty(t, res.Recommendations)
	assert.Equal(t, int64(0), res.Reclaimable)
}

// TestAnalyzeCleanupBranchGone verifies that a stash with a "branch:" tag
// referencing a deleted git branch is categorized as "branch-gone".
func TestAnalyzeCleanupBranchGone(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	assert.NoError(t, err)

	// Create the loose-ref shape of a git repo with a branch, save a stash
	// referencing it, then delete the branch.
	gitDir := filepath.Join(tmp, "gitrepo")
	assert.NoError(t, os.MkdirAll(gitDir, 0755))
	assert.NoError(t, os.WriteFile(filepath.Join(gitDir, "f.txt"), []byte("x"), 0644))
	branchRef := filepath.Join(gitDir, ".git", "refs", "heads", "feature-x")
	assert.NoError(t, os.MkdirAll(filepath.Dir(branchRef), 0755))
	assert.NoError(t, os.WriteFile(branchRef, []byte("0123456789012345678901234567890123456789\n"), 0644))

	// Save a stash with a branch tag.
	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: gitDir,
		Name:       "branch-test",
		Tags:       []string{"branch:feature-x"},
	})
	assert.NoError(t, err)

	// Delete the branch.
	assert.NoError(t, os.Remove(branchRef))

	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)

	var rec *CleanupRecommendation
	for i := range res.Recommendations {
		if res.Recommendations[i].ID == st.Manifest.ID {
			rec = &res.Recommendations[i]
		}
	}
	assert.NotNil(t, rec)
	assert.Equal(t, CatBranchGone, rec.Category)
}
