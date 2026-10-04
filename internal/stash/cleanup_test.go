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
// reclaimable, even when its TTL has expired.
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

	// Analyze: the keep tag outranks every category, so an expired but pinned
	// stash is retained and contributes nothing to the reclaimable total.
	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	assert.Equal(t, CatKeep, res.Recommendations[0].Category)
	assert.Contains(t, res.Recommendations[0].Reason, "keep tag")
	assert.Equal(t, int64(0), res.Reclaimable)

	// Analysis never deletes; verify the stash still exists.
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

func saveForCleanup(t *testing.T, mgr *Manager, root, name string, opts SaveOptions, files map[string]string) (*Stash, string) {
	t.Helper()
	src := filepath.Join(root, name)
	assert.NoError(t, os.MkdirAll(src, 0o755))
	for file, body := range files {
		assert.NoError(t, os.WriteFile(filepath.Join(src, file), []byte(body), 0o644))
	}
	opts.SourcePath = src
	st, err := mgr.Save(context.Background(), &opts)
	assert.NoError(t, err)
	return st, src
}

func TestAnalyzeCleanupKeepTagProtectsEveryCategory(t *testing.T) {
	root := t.TempDir()
	mgr, err := NewManager(filepath.Join(root, "vault"))
	assert.NoError(t, err)

	pinned, src := saveForCleanup(t, mgr, root, "pinned", SaveOptions{Tags: []string{"keep"}}, map[string]string{"f.txt": "x"})
	custom, customSrc := saveForCleanup(t, mgr, root, "custom", SaveOptions{Tags: []string{"pinned"}}, map[string]string{"f.txt": "y"})
	assert.NoError(t, os.RemoveAll(src))
	assert.NoError(t, os.RemoveAll(customSrc))

	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{KeepTag: "pinned"})
	assert.NoError(t, err)
	assert.Equal(t, 2, res.ByCategory[CatKeep])
	assert.Equal(t, int64(0), res.Reclaimable)
	for _, rec := range res.Recommendations {
		assert.Equal(t, CatKeep, rec.Category, rec.ID)
	}
	assert.True(t, mgr.Exists(pinned.Manifest.ID))
	assert.True(t, mgr.Exists(custom.Manifest.ID))

	// Without the custom tag the custom-pinned stash is an ordinary orphan again.
	res, err = mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 1, res.ByCategory[CatOrphaned])
	assert.Equal(t, 1, res.ByCategory[CatKeep])
}

func TestAnalyzeCleanupReportsRunEvidenceAsLastCopyNotOrphaned(t *testing.T) {
	root := t.TempDir()
	mgr, err := NewManager(filepath.Join(root, "vault"))
	assert.NoError(t, err)

	byTool, toolSrc := saveForCleanup(t, mgr, root, "run-a", SaveOptions{Tool: "cairntrace"}, map[string]string{"report.html": "<p>a</p>"})
	byBundle, bundleSrc := saveForCleanup(t, mgr, root, "run-b", SaveOptions{}, map[string]string{
		"artifact-manifest.json": `{}`,
	})
	assert.Equal(t, "cairntrace-run", byBundle.Manifest.BundleType)
	generic, genericSrc := saveForCleanup(t, mgr, root, "scratch", SaveOptions{Tool: "scratchpad"}, map[string]string{"f.txt": "z"})
	live, _ := saveForCleanup(t, mgr, root, "run-live", SaveOptions{Tool: "glyphrun"}, map[string]string{"report.html": "<p>live</p>"})
	for _, src := range []string{toolSrc, bundleSrc, genericSrc} {
		assert.NoError(t, os.RemoveAll(src))
	}

	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	got := map[string]CleanupRecommendation{}
	for _, rec := range res.Recommendations {
		got[rec.ID] = rec
	}
	assert.Equal(t, CatEvidence, got[byTool.Manifest.ID].Category)
	assert.Contains(t, got[byTool.Manifest.ID].Reason, "last copy")
	assert.Equal(t, CatOrphaned, got[generic.Manifest.ID].Category)
	assert.Equal(t, CatKeep, got[live.Manifest.ID].Category, "run evidence with a live source is not a candidate")
	assert.Equal(t, CatEvidence, got[byBundle.Manifest.ID].Category, "bundle detection alone marks run evidence")

	assert.Equal(t, 2, res.ByCategory[CatEvidence])
	assert.Equal(t, 1, res.ByCategory[CatOrphaned])
	assert.Equal(t, got[generic.Manifest.ID].Size, res.Reclaimable, "only the generic orphan is reclaimable")
}

func TestIsRunEvidenceUsesBundleTypeOrTool(t *testing.T) {
	assert.True(t, isRunEvidence(&manifest.Manifest{BundleType: "cairntrace-run"}))
	assert.True(t, isRunEvidence(&manifest.Manifest{BundleType: "glyphrun-run"}))
	assert.True(t, isRunEvidence(&manifest.Manifest{Tool: "cairntrace"}))
	assert.True(t, isRunEvidence(&manifest.Manifest{Tool: "glyphrun"}))
	assert.False(t, isRunEvidence(&manifest.Manifest{Tool: "codemap", BundleType: "generic"}))
	assert.False(t, isRunEvidence(&manifest.Manifest{}))
}

func TestAnalyzeCleanupExpiredTTLStillOutranksEvidence(t *testing.T) {
	root := t.TempDir()
	mgr, err := NewManager(filepath.Join(root, "vault"))
	assert.NoError(t, err)
	st, src := saveForCleanup(t, mgr, root, "run", SaveOptions{Tool: "cairntrace", TTL: "1s"}, map[string]string{"report.html": "x"})
	assert.NoError(t, os.RemoveAll(src))
	time.Sleep(2 * time.Second)

	res, err := mgr.AnalyzeCleanup(context.Background(), CleanupOptions{})
	assert.NoError(t, err)
	// An explicit TTL is retention intent and still outranks evidence retention.
	assert.Equal(t, CatExpired, res.Recommendations[0].Category)
	assert.Equal(t, st.Manifest.ID, res.Recommendations[0].ID)
}
