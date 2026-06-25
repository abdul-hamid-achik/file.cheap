package cleanup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/stretchr/testify/assert"
)

func TestAnalyzeCacheToolSourceGone(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := stash.NewManager(tmp)
	assert.NoError(t, err)

	// Create a source dir, save, then delete it so source is "gone".
	srcDir := filepath.Join(tmp, "cache-src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	st, err := mgr.Save(t.Context(), &stash.SaveOptions{
		SourcePath: srcDir,
		Tool:       "codemap",
		Tags:       []string{"codemap-snapshot"},
	})
	assert.NoError(t, err)
	_ = os.RemoveAll(srcDir) // make source path "gone"

	hashIndex := map[string][]*stash.Stash{}
	c := analyze(st, hashIndex, "keep")

	// Cache tool (+25) + source path gone (+35) = 60 -> drop
	assert.Equal(t, 60, c.Score)
	assert.Equal(t, "drop", c.Verdict)
}

func TestAnalyzeEvidenceToolProtected(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "evidence-source")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))

	mgr, err := stash.NewManager(tmp)
	assert.NoError(t, err)

	st, err := mgr.Save(t.Context(), &stash.SaveOptions{
		SourcePath: srcDir, // source exists, so no +35
		Tool:       "vidtrace",
	})
	assert.NoError(t, err)

	hashIndex := map[string][]*stash.Stash{}
	c := analyze(st, hashIndex, "keep")

	// Evidence tool (-30) only signal -> score 0 -> keep
	assert.Equal(t, 0, c.Score)
	assert.Equal(t, "keep", c.Verdict)
}

func TestAnalyzeKeepTagProtected(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "cache-src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))

	mgr, err := stash.NewManager(tmp)
	assert.NoError(t, err)

	st, err := mgr.Save(t.Context(), &stash.SaveOptions{
		SourcePath: srcDir,
		Tool:       "codemap",        // +25
		Tags:       []string{"keep"}, // -50
	})
	assert.NoError(t, err)

	hashIndex := map[string][]*stash.Stash{}
	c := analyze(st, hashIndex, "keep")

	// Cache tool (+25) + keep tag (-50) = -25 -> clamped to 0 -> keep
	assert.Equal(t, 0, c.Score)
	assert.Equal(t, "keep", c.Verdict)
}

func TestAnalyzeExpiredTTL(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))

	mgr, err := stash.NewManager(tmp)
	assert.NoError(t, err)

	st, err := mgr.Save(t.Context(), &stash.SaveOptions{
		SourcePath: srcDir,    // source exists
		Tool:       "codemap", // +25
		TTL:        "1h",
	})
	assert.NoError(t, err)

	// Manually set expires_at to the past to simulate expiry.
	st.Manifest.ExpiresAt = time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	assert.NoError(t, st.Manifest.Save(filepath.Join(tmp, st.Manifest.ID)))

	hashIndex := map[string][]*stash.Stash{}
	c := analyze(st, hashIndex, "keep")

	// Cache tool (+25) + expired TTL (+40) = 65 -> drop
	assert.Equal(t, 65, c.Score)
	assert.Equal(t, "drop", c.Verdict)
}

func TestAnalyzeContentDedup(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	// Create a file so both stashes have the same content hash.
	assert.NoError(t, os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("same content"), 0644))

	mgr, err := stash.NewManager(tmp)
	assert.NoError(t, err)

	// Save two stashes with identical content but different names.
	st1, err := mgr.Save(t.Context(), &stash.SaveOptions{SourcePath: srcDir, Name: "first", Tool: "codemap"})
	assert.NoError(t, err)
	// Manually set st1's CreatedAt to 1 second earlier (RFC3339 is second-level
	// precision, so a 10ms sleep wouldn't change it).
	st1.Manifest.CreatedAt = time.Now().Add(-1 * time.Second).UTC().Format(time.RFC3339)
	assert.NoError(t, st1.Manifest.Save(filepath.Join(tmp, st1.Manifest.ID)))
	st2, err := mgr.Save(t.Context(), &stash.SaveOptions{SourcePath: srcDir, Name: "second", Tool: "codemap"})
	assert.NoError(t, err)

	// Build hash index with both stashes.
	hashIndex := map[string][]*stash.Stash{}
	for _, st := range []*stash.Stash{st1, st2} {
		if st.Manifest.ContentHash != "" {
			hashIndex[st.Manifest.ContentHash] = append(hashIndex[st.Manifest.ContentHash], st)
		}
	}

	// st1 is older, so it should get the dedup bonus.
	c1 := analyze(st1, hashIndex, "keep")
	c2 := analyze(st2, hashIndex, "keep")

	// st1: cache tool (+25) + dedup (+20) = 45 -> review
	// st2: cache tool (+25) = 25 -> keep (below review threshold)
	assert.Equal(t, 45, c1.Score)
	assert.Equal(t, "review", c1.Verdict)
	assert.Equal(t, 25, c2.Score)
	assert.Equal(t, "keep", c2.Verdict)
}

func TestAnalyzeKeepTagHardFloor(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "cache-src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))

	mgr, err := stash.NewManager(tmp)
	assert.NoError(t, err)

	// Save a stash with keep tag, cache tool, and expired TTL — this would
	// score 25+40-50=15, but with source gone it'd be 25+35+40-50=50 (review).
	// The hard floor ensures it's always "keep" regardless.
	st, err := mgr.Save(t.Context(), &stash.SaveOptions{
		SourcePath: srcDir,
		Tool:       "codemap", // +25
		TTL:        "1h",
		Tags:       []string{"keep"}, // -50 + hard floor
	})
	assert.NoError(t, err)
	_ = os.RemoveAll(srcDir) // source gone (+35)

	// Manually expire the TTL.
	st.Manifest.ExpiresAt = time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	assert.NoError(t, st.Manifest.Save(filepath.Join(tmp, st.Manifest.ID)))

	hashIndex := map[string][]*stash.Stash{}
	c := analyze(st, hashIndex, "keep")

	// Score = 25+35+40-50 = 50, but keep tag is a hard floor -> verdict = keep.
	assert.Equal(t, 50, c.Score)
	assert.Equal(t, "keep", c.Verdict, "keep tag should be a hard floor, not just -50")
}

func TestRunDryRunNoDrop(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := stash.NewManager(tmp)
	assert.NoError(t, err)

	// Save a stash with a source path that will be deleted.
	srcDir := filepath.Join(tmp, "cache-src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	_, err = mgr.Save(t.Context(), &stash.SaveOptions{
		SourcePath: srcDir,
		Tool:       "codemap", // +25
	})
	assert.NoError(t, err)
	_ = os.RemoveAll(srcDir) // make source "gone" (+35)

	// Dry-run: should report candidates but not drop.
	result, err := Run(t.Context(), mgr, nil, Options{KeepTag: "keep"})
	assert.NoError(t, err)
	assert.False(t, result.Applied)
	assert.NotEmpty(t, result.Candidates)
	assert.Empty(t, result.Dropped)

	// Stash should still exist.
	stashes, _ := mgr.List(t.Context(), "")
	assert.Len(t, stashes, 1)
}

func TestRunApplyDropsHighConfidence(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := stash.NewManager(tmp)
	assert.NoError(t, err)

	// Save a stash with a source path that will be deleted.
	srcDir := filepath.Join(tmp, "cache-src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	_, err = mgr.Save(t.Context(), &stash.SaveOptions{
		SourcePath: srcDir,
		Tool:       "codemap", // +25
	})
	assert.NoError(t, err)
	_ = os.RemoveAll(srcDir) // make source "gone" (+35)

	// Apply: should drop the stash (score 60 = drop).
	result, err := Run(t.Context(), mgr, nil, Options{Apply: true, KeepTag: "keep"})
	assert.NoError(t, err)
	assert.True(t, result.Applied)
	assert.NotEmpty(t, result.Dropped)

	// Stash should be gone.
	stashes, _ := mgr.List(t.Context(), "")
	assert.Empty(t, stashes)
}

func TestRunApplyRespectsKeepTag(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := stash.NewManager(tmp)
	assert.NoError(t, err)

	// Save a stash with the keep tag.
	srcDir := filepath.Join(tmp, "cache-src")
	assert.NoError(t, os.MkdirAll(srcDir, 0755))
	_, err = mgr.Save(t.Context(), &stash.SaveOptions{
		SourcePath: srcDir,
		Tool:       "codemap",        // +25
		Tags:       []string{"keep"}, // -50
	})
	assert.NoError(t, err)
	_ = os.RemoveAll(srcDir) // make source "gone" (+35)

	// Apply with keep tag: should NOT drop (25+35-50=10, below 30 = keep).
	result, err := Run(t.Context(), mgr, nil, Options{Apply: true, KeepTag: "keep"})
	assert.NoError(t, err)
	assert.True(t, result.Applied)
	assert.Empty(t, result.Dropped) // keep tag protects it

	// Stash should still exist.
	stashes, _ := mgr.List(t.Context(), "")
	assert.Len(t, stashes, 1)
}
