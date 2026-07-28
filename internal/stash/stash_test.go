package stash

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
)

func TestSaveAndList(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}

	// Create a source directory
	srcDir := filepath.Join(tmp, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "b.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "test-stash",
		Tags:       []string{"bug", "test"},
		Tool:       "manual",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if st.Manifest == nil {
		t.Fatal("manifest should not be nil")
	}
	if st.Manifest.ID == "" {
		t.Error("ID should not be empty")
	}
	if st.Manifest.FileCount != 2 {
		t.Errorf("FileCount = %d, want 2", st.Manifest.FileCount)
	}
	if st.Manifest.BundleType != "generic" {
		t.Errorf("BundleType = %s, want generic", st.Manifest.BundleType)
	}

	// List stashes
	stashes, err := mgr.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(stashes) != 1 {
		t.Fatalf("List count = %d, want 1", len(stashes))
	}

	// List with tag filter
	stashes, err = mgr.List(context.Background(), "bug")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(stashes) != 1 {
		t.Fatalf("List with tag 'bug' = %d, want 1", len(stashes))
	}

	// List with non-matching tag
	stashes, err = mgr.List(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(stashes) != 0 {
		t.Errorf("List with nonexistent tag = %d, want 0", len(stashes))
	}
}

func TestSaveDetectsMonitorIncident(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(tmp, "incident")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"manifest.json":     `{"kind":"monitor.incident","schema_version":"1","diagnosis":{"summary":"worker memory leak"}}`,
		"snapshot.json":     `{}`,
		"profile.json":      `{}`,
		"process.json":      `{"runtime":"nodejs"}`,
		"correlations.json": `{"matches":[]}`,
		"semantic.json":     `{"hits":[]}`,
	} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: source,
		Name:       "monitor incident",
		Tool:       "monitor",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if st.Manifest.BundleType != "monitor.incident" {
		t.Fatalf("BundleType = %q, want monitor.incident", st.Manifest.BundleType)
	}
	if st.Manifest.Tool != "monitor" {
		t.Fatalf("Tool = %q, want monitor", st.Manifest.Tool)
	}
}

func TestRestore(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}

	// Create a source directory
	srcDir := filepath.Join(tmp, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "restore-test",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Restore to a target directory
	target := filepath.Join(tmp, "restored")
	res, err := mgr.Restore(context.Background(), st.Manifest.ID, target)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !res.Verified {
		t.Errorf("restore not verified, mismatches: %v", res.Mismatches)
	}
	if res.FileCount != 1 {
		t.Errorf("FileCount = %d, want 1", res.FileCount)
	}

	// Verify restored file
	restored := filepath.Join(target, "file.txt")
	data, err := os.ReadFile(restored)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("restored content = %q, want 'content'", string(data))
	}
}

func TestCompressAndRestore(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "source")
	if err := os.MkdirAll(filepath.Join(srcDir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "sub", "b.txt"), []byte("nested content"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: srcDir, Name: "compress-test"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	id := st.Manifest.ID

	res, err := mgr.Compress(context.Background(), id, "zstd")
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if res.CompressedSize <= 0 {
		t.Errorf("CompressedSize = %d, want > 0", res.CompressedSize)
	}

	// The extracted content tree must be removed to reclaim space.
	if _, err := os.Stat(filepath.Join(mgr.StashDir(id), "content")); !os.IsNotExist(err) {
		t.Error("content/ tree should be removed after compress")
	}
	// The archive must exist.
	if _, err := os.Stat(filepath.Join(mgr.StashDir(id), "content.tar.zst")); err != nil {
		t.Errorf("archive should exist after compress: %v", err)
	}

	// Restore from the compressed archive and verify integrity.
	target := filepath.Join(tmp, "restored")
	rres, err := mgr.Restore(context.Background(), id, target)
	if err != nil {
		t.Fatalf("Restore from archive: %v", err)
	}
	if !rres.Verified {
		t.Errorf("restore from archive not verified: %v", rres.Mismatches)
	}
	data, err := os.ReadFile(filepath.Join(target, "sub", "b.txt"))
	if err != nil {
		t.Fatalf("read restored nested file: %v", err)
	}
	if string(data) != "nested content" {
		t.Errorf("restored content = %q, want 'nested content'", string(data))
	}
}

func TestValidateArtifactEntrypointForExtractedAndCompressedStashes(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "source")
	if err := os.MkdirAll(filepath.Join(src, "reports"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "reports", "run.json"), []byte(`{"status":"passed"}`), 0644); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src})
	if err != nil {
		t.Fatal(err)
	}

	if err := mgr.ValidateArtifactEntrypoint(context.Background(), st, "reports/run.json"); err != nil {
		t.Fatalf("ValidateArtifactEntrypoint extracted: %v", err)
	}
	if err := mgr.ValidateArtifactEntrypoint(context.Background(), st, "reports/missing.json"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("ValidateArtifactEntrypoint missing error = %v", err)
	}

	if _, err := mgr.Compress(context.Background(), st.Manifest.ID, "zstd"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(st.Dir, "content")); !os.IsNotExist(err) {
		t.Fatalf("compressed stash still has content tree: %v", err)
	}
	if err := mgr.ValidateArtifactEntrypoint(context.Background(), st, "reports/run.json"); err != nil {
		t.Fatalf("ValidateArtifactEntrypoint compressed: %v", err)
	}
	if err := mgr.ValidateArtifactEntrypoint(context.Background(), st, "reports/missing.json"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("ValidateArtifactEntrypoint compressed missing error = %v", err)
	}

	// Early manifests can omit the per-file list while retaining file_count.
	// Keep validating those stashes by inspecting their archive.
	st.Manifest.Files = nil
	if err := mgr.ValidateArtifactEntrypoint(context.Background(), st, "reports/run.json"); err != nil {
		t.Fatalf("ValidateArtifactEntrypoint legacy manifest: %v", err)
	}
}

func TestValidateArtifactEntrypointRejectsEmptyStashWithoutArchiveScan(t *testing.T) {
	mgr, err := NewManager(filepath.Join(t.TempDir(), "vault"))
	if err != nil {
		t.Fatal(err)
	}
	st := &Stash{
		Manifest: &manifest.Manifest{FileCount: 0},
		Dir:      t.TempDir(),
	}
	err = mgr.ValidateArtifactEntrypoint(context.Background(), st, "run.json")
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("ValidateArtifactEntrypoint empty stash error = %v", err)
	}
}

func TestDrop(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "x.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "drop-test",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if !mgr.Exists(st.Manifest.ID) {
		t.Error("stash should exist")
	}

	if err := mgr.Drop(context.Background(), st.Manifest.ID); err != nil {
		t.Fatalf("Drop: %v", err)
	}

	if mgr.Exists(st.Manifest.ID) {
		t.Error("stash should not exist after drop")
	}
}

func TestDropHonorsPreCancelledContext(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(tmp, "source.txt")
	if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: source})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mgr.Drop(ctx, st.Manifest.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Drop error = %v, want context.Canceled", err)
	}
	if !mgr.Exists(st.Manifest.ID) {
		t.Fatal("pre-cancelled Drop deleted the stash")
	}
}

func TestMetadataIndexSync(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: srcDir, Name: "idx", Tags: []string{"x"}})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The SQLite index should be populated by Save.
	count, total := mgr.Stats(context.Background())
	if count != 1 {
		t.Errorf("Stats count = %d, want 1", count)
	}
	if total != st.Manifest.TotalSize {
		t.Errorf("Stats total = %d, want %d", total, st.Manifest.TotalSize)
	}

	// And cleared by Drop.
	if err := mgr.Drop(context.Background(), st.Manifest.ID); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if count, _ := mgr.Stats(context.Background()); count != 0 {
		t.Errorf("Stats count after drop = %d, want 0", count)
	}
}

func TestParseSince(t *testing.T) {
	now := time.Now()
	cases := []struct {
		in   string
		want time.Duration
		date string
		ok   bool
	}{
		{in: "24h", want: 24 * time.Hour, ok: true},
		{in: "90m", want: 90 * time.Minute, ok: true},
		{in: "7d", want: 7 * 24 * time.Hour, ok: true},
		{in: "2w", want: 14 * 24 * time.Hour, ok: true},
		{in: "2026-06-01", date: "2026-06-01", ok: true},
		{in: "garbage", ok: false},
		{in: "5x", ok: false},
	}
	for _, c := range cases {
		got, err := ParseSince(c.in)
		if !c.ok {
			if err == nil {
				t.Errorf("ParseSince(%q) = no error, want error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseSince(%q) error: %v", c.in, err)
			continue
		}
		if c.date != "" {
			want, _ := time.Parse("2006-01-02", c.date)
			if !got.Equal(want) {
				t.Errorf("ParseSince(%q) = %v, want %v", c.in, got, want)
			}
			continue
		}
		if d := now.Sub(got) - c.want; d < -2*time.Second || d > 2*time.Second {
			t.Errorf("ParseSince(%q) cutoff age = %v, want ~%v", c.in, now.Sub(got), c.want)
		}
	}
}

func TestListFiltered(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	mk := func(name, tool string) {
		src := filepath.Join(tmp, "src-"+name)
		_ = os.MkdirAll(src, 0755)
		_ = os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0644)
		if _, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, Name: name, Tool: tool, Tags: []string{tool}}); err != nil {
			t.Fatal(err)
		}
	}
	mk("one", "vidtrace")
	mk("two", "manual")

	if got, _ := mgr.ListFiltered(context.Background(), ListOptions{Tool: "vidtrace"}); len(got) != 1 {
		t.Errorf("filter by tool = %d, want 1", len(got))
	}
	if got, _ := mgr.ListFiltered(context.Background(), ListOptions{Tag: "manual"}); len(got) != 1 {
		t.Errorf("filter by tag = %d, want 1", len(got))
	}
	if got, _ := mgr.ListFiltered(context.Background(), ListOptions{Limit: 1}); len(got) != 1 {
		t.Errorf("limit = %d, want 1", len(got))
	}
	if got, _ := mgr.ListFiltered(context.Background(), ListOptions{Since: time.Now().Add(-time.Hour)}); len(got) != 2 {
		t.Errorf("since 1h ago = %d, want 2", len(got))
	}
	if got, _ := mgr.ListFiltered(context.Background(), ListOptions{Since: time.Now().Add(time.Hour)}); len(got) != 0 {
		t.Errorf("since 1h future = %d, want 0", len(got))
	}
}

func TestListFilteredMultiTag(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	// Stash A: codemap-index + repo:abc. Stash B: codemap-index + repo:def.
	// Stash C: codemap-index only.
	for _, tags := range [][]string{
		{"codemap-index", "repo:abc"},
		{"codemap-index", "repo:def"},
		{"codemap-index"},
	} {
		src := filepath.Join(tmp, "src-"+strings.Join(tags, "_"))
		_ = os.MkdirAll(src, 0755)
		_ = os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0644)
		if _, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, Tags: tags}); err != nil {
			t.Fatal(err)
		}
	}

	// AND: codemap-index + repo:abc -> exactly one (stash A).
	if got, _ := mgr.ListFiltered(context.Background(), ListOptions{Tags: []string{"codemap-index", "repo:abc"}}); len(got) != 1 {
		t.Errorf("multi-tag AND {codemap-index, repo:abc} = %d, want 1", len(got))
	}
	// AND with a tag no stash has -> 0.
	if got, _ := mgr.ListFiltered(context.Background(), ListOptions{Tags: []string{"codemap-index", "repo:zzz"}}); len(got) != 0 {
		t.Errorf("multi-tag AND {codemap-index, repo:zzz} = %d, want 0", len(got))
	}
	// Single tag via the legacy Tag field still works -> all 3.
	if got, _ := mgr.ListFiltered(context.Background(), ListOptions{Tag: "codemap-index"}); len(got) != 3 {
		t.Errorf("legacy single Tag = %d, want 3", len(got))
	}
	// Legacy Tag + Tags merged (AND) -> 1.
	if got, _ := mgr.ListFiltered(context.Background(), ListOptions{Tag: "codemap-index", Tags: []string{"repo:def"}}); len(got) != 1 {
		t.Errorf("Tag+Tags merge = %d, want 1", len(got))
	}
}

func TestVacuum(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "src")
	_ = os.MkdirAll(src, 0755)
	_ = os.WriteFile(filepath.Join(src, "a.txt"), []byte("x"), 0644)

	keep, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, Name: "keep"})
	if err != nil {
		t.Fatal(err)
	}
	gone, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, Name: "gone"})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a stash directory removed outside `drop` (DB row remains).
	if err := os.RemoveAll(mgr.StashDir(gone.Manifest.ID)); err != nil {
		t.Fatal(err)
	}

	dropped := []string{}
	res, err := mgr.Vacuum(context.Background(), func(id string) error {
		dropped = append(dropped, id)
		return nil
	})
	if err != nil {
		t.Fatalf("Vacuum: %v", err)
	}
	if res.OnDisk != 1 {
		t.Errorf("OnDisk = %d, want 1", res.OnDisk)
	}
	if res.OrphanedRows != 1 || len(res.Orphans) != 1 || res.Orphans[0] != gone.Manifest.ID {
		t.Errorf("Orphans = %v, want [%s]", res.Orphans, gone.Manifest.ID)
	}
	if len(dropped) != 1 || dropped[0] != gone.Manifest.ID {
		t.Errorf("dropIndex called with %v, want [%s]", dropped, gone.Manifest.ID)
	}

	// The surviving stash must still be listed; the orphan must not.
	count, _ := mgr.Stats(context.Background())
	if count != 1 {
		t.Errorf("Stats count after vacuum = %d, want 1", count)
	}
	_ = keep
}

func TestSaveSingleFile(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}

	// Create a single source file
	srcFile := filepath.Join(tmp, "single.txt")
	if err := os.WriteFile(srcFile, []byte("single file content"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcFile,
		Name:       "single-file",
	})
	if err != nil {
		t.Fatalf("Save single file: %v", err)
	}

	if st.Manifest.FileCount != 1 {
		t.Errorf("FileCount = %d, want 1", st.Manifest.FileCount)
	}
}

// TestSaveDirWithDanglingSymlink verifies a directory containing a dangling
// symlink can still be stashed (the link is preserved, not dereferenced).
// Regression test for "Save aborts on a source dir with a dangling symlink".
func TestSaveDirWithDanglingSymlink(t *testing.T) {
	root := t.TempDir()
	mgr, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "ok.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/no/such/target", filepath.Join(src, "broken")); err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, Name: "with-link"})
	if err != nil {
		t.Fatalf("Save with a dangling symlink should succeed, got: %v", err)
	}
	if st.Manifest.FileCount < 2 {
		t.Errorf("FileCount = %d, want >= 2 (file + symlink)", st.Manifest.FileCount)
	}

	// The recreated symlink should still be a (dangling) link in the stash.
	link := filepath.Join(st.Dir, "content", "broken")
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected a symlink at %s (err %v)", link, err)
	}
}

// TestStashIDTraversalRejected verifies that a path-traversal stash id cannot
// escape the stash root — in particular that Drop cannot os.RemoveAll a
// directory outside the root. Regression test for the high-severity MCP
// arbitrary-deletion finding.
func TestStashIDTraversalRejected(t *testing.T) {
	root := t.TempDir()
	mgr, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}

	// A victim directory OUTSIDE the stash root that a traversal id would target.
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim")
	if err := os.MkdirAll(victim, 0755); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, victim) // e.g. "../<tmp>/victim" — has separators/..
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(root, "fcheap.db")
	if err := os.WriteFile(metadataPath, []byte("metadata"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, badID := range []string{rel, "../../etc", "..", ".", "a/b", "", "fcheap.db", "Fcheap.db", "fcheap.db-wal", "fcheap.veclite"} {
		if mgr.Exists(badID) {
			t.Errorf("Exists(%q) = true, want false", badID)
		}
		if err := mgr.Drop(context.Background(), badID); err == nil {
			t.Errorf("Drop(%q) succeeded, want rejection", badID)
		}
		if _, err := mgr.Restore(context.Background(), badID, ""); err == nil {
			t.Errorf("Restore(%q) succeeded, want rejection", badID)
		}
		if _, err := mgr.Info(context.Background(), badID); err == nil {
			t.Errorf("Info(%q) succeeded, want rejection", badID)
		}
		if _, err := mgr.Compress(context.Background(), badID, "zstd"); err == nil {
			t.Errorf("Compress(%q) succeeded, want rejection", badID)
		}
	}

	// The victim must be untouched — no traversal Drop deleted it.
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("victim directory was deleted via traversal: %v", err)
	}
	if data, err := os.ReadFile(metadataPath); err != nil || string(data) != "metadata" {
		t.Errorf("vault metadata was deleted via Drop: data=%q err=%v", data, err)
	}
}

func TestSaveHonorsCancelledContextDuringCopy(t *testing.T) {
	root := t.TempDir()
	mgr, err := NewManager(filepath.Join(root, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "data.txt"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := mgr.Save(ctx, &SaveOptions{SourcePath: source, Name: "cancelled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save error = %v, want context.Canceled", err)
	}
	stashes, err := mgr.ListFiltered(context.Background(), ListOptions{IncludeExpired: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(stashes) != 0 {
		t.Fatalf("cancelled save left %d stash(es)", len(stashes))
	}
}

func TestRestoreRejectsCorruptManifestBeforeWriting(t *testing.T) {
	root := t.TempDir()
	mgr, err := NewManager(filepath.Join(root, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: source, Name: "corrupt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Dir, "manifest.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "restore-target")
	if _, err := mgr.Restore(context.Background(), st.Manifest.ID, target); err == nil {
		t.Fatal("Restore succeeded with a corrupt manifest")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("restore wrote target before validating manifest: %v", err)
	}
}

func TestOperationsRejectManifestDirectoryIDMismatch(t *testing.T) {
	root := t.TempDir()
	mgr, err := NewManager(filepath.Join(root, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: source, Name: "identity"})
	if err != nil {
		t.Fatal(err)
	}
	id := st.Manifest.ID
	st.Manifest.ID = "different-stash"
	if err := st.Manifest.Save(st.Dir); err != nil {
		t.Fatal(err)
	}

	if mgr.Exists(id) {
		t.Fatal("Exists accepted a manifest whose ID does not match its directory")
	}
	if _, err := mgr.Info(context.Background(), id); err == nil {
		t.Fatal("Info accepted a manifest whose ID does not match its directory")
	}
	if _, err := mgr.Compress(context.Background(), id, "zstd"); err == nil {
		t.Fatal("Compress accepted a manifest whose ID does not match its directory")
	}
	if err := mgr.SetExpiry(context.Background(), id, "1h"); err == nil {
		t.Fatal("SetExpiry accepted a manifest whose ID does not match its directory")
	}
	target := filepath.Join(root, "restore-target")
	if _, err := mgr.Restore(context.Background(), id, target); err == nil {
		t.Fatal("Restore accepted a manifest whose ID does not match its directory")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("restore wrote target before checking manifest identity: %v", err)
	}
}

func TestOperationsRejectSymlinkedStashDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	root := t.TempDir()
	mgr, err := NewManager(filepath.Join(root, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	id := "planted-stash"
	outside := t.TempDir()
	content := filepath.Join(outside, "content")
	if err := os.Mkdir(content, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(content, "outside.txt"), []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	man := manifest.New(id, outside)
	if err := man.ScanFiles(content); err != nil {
		t.Fatal(err)
	}
	if err := man.Save(outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, mgr.StashDir(id)); err != nil {
		t.Fatal(err)
	}

	if mgr.Exists(id) {
		t.Fatal("Exists followed a symlinked stash directory")
	}
	if _, err := mgr.Info(context.Background(), id); err == nil {
		t.Fatal("Info followed a symlinked stash directory")
	}
	if _, err := mgr.Compress(context.Background(), id, "zstd"); err == nil {
		t.Fatal("Compress followed a symlinked stash directory")
	}
	if err := mgr.SetExpiry(context.Background(), id, "1h"); err == nil {
		t.Fatal("SetExpiry followed a symlinked stash directory")
	}
	if _, err := mgr.Restore(context.Background(), id, filepath.Join(root, "target")); err == nil {
		t.Fatal("Restore followed a symlinked stash directory")
	}
	if data, err := os.ReadFile(filepath.Join(content, "outside.txt")); err != nil || string(data) != "untouched" {
		t.Fatalf("outside stash content changed: data=%q err=%v", data, err)
	}
}

func TestInfoRejectsSymlinkedManifest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	root := t.TempDir()
	mgr, err := NewManager(filepath.Join(root, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	id := "manifest-link"
	stashDir := mgr.StashDir(id)
	if err := os.Mkdir(stashDir, 0700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	man := manifest.New(id, outside)
	if err := man.Save(outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "manifest.json"), filepath.Join(stashDir, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if mgr.Exists(id) {
		t.Fatal("Exists followed a symlinked manifest")
	}
	if _, err := mgr.Info(context.Background(), id); err == nil {
		t.Fatal("Info followed a symlinked manifest")
	}
}

// TestRestoreDropsEscapingSymlinks verifies that symlinks which would resolve
// outside the restore target are not materialized on restore (matching the
// archive path), even though Save preserves them verbatim in the vault.
func TestRestoreDropsEscapingSymlinks(t *testing.T) {
	root := t.TempDir()
	mgr, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "ok.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(src, "abslink")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../escape", filepath.Join(src, "relesc")); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, Name: "links"})
	if err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	if _, err := mgr.Restore(context.Background(), st.Manifest.ID, target); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	for _, bad := range []string{"abslink", "relesc"} {
		if _, err := os.Lstat(filepath.Join(target, bad)); !os.IsNotExist(err) {
			t.Errorf("escaping symlink %q should be dropped on restore (err %v)", bad, err)
		}
	}
	if _, err := os.Stat(filepath.Join(target, "ok.txt")); err != nil {
		t.Errorf("safe file ok.txt should be restored: %v", err)
	}
}

func TestRestoreRejectsPlantedSymlinkParent(t *testing.T) {
	root := t.TempDir()
	mgr, err := NewManager(filepath.Join(root, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "sub", "file.txt"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: source, Name: "parent-link"})
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "target")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(target, "sub")); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Restore(context.Background(), st.Manifest.ID, target); err == nil ||
		errors.Unwrap(err) == nil || !strings.Contains(errors.Unwrap(err).Error(), "symlink path component") {
		t.Fatalf("Restore error = %v (cause %v), want symlink-component rejection", err, errors.Unwrap(err))
	}
	if _, err := os.Stat(filepath.Join(outside, "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("restore escaped through a planted parent symlink: %v", err)
	}
}

func TestRestoreVerifiesSafeSymlinks(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		name := "extracted"
		if compressed {
			name = "compressed"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			mgr, err := NewManager(filepath.Join(root, "vault"))
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(root, "source")
			if err := os.MkdirAll(source, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, "real.txt"), []byte("payload"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("real.txt", filepath.Join(source, "link.txt")); err != nil {
				t.Fatal(err)
			}
			st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: source, Name: name})
			if err != nil {
				t.Fatal(err)
			}
			if compressed {
				if _, err := mgr.Compress(context.Background(), st.Manifest.ID, "zstd"); err != nil {
					t.Fatal(err)
				}
			}
			res, err := mgr.Restore(context.Background(), st.Manifest.ID, filepath.Join(root, "target"))
			if err != nil {
				t.Fatal(err)
			}
			if !res.Verified || len(res.Mismatches) != 0 {
				t.Fatalf("safe symlink did not verify: %+v", res)
			}
		})
	}
}

func TestSaveRejectsVaultPathRelationships(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "vault")
	mgr, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}

	inside := filepath.Join(root, "source")
	if err := os.MkdirAll(inside, 0755); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]string{
		"equal":    root,
		"inside":   inside,
		"ancestor": tmp,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: source, NoScan: true}); err == nil {
				t.Fatalf("Save(%q) succeeded, want vault-overlap rejection", source)
			}
		})
	}
}

func TestSaveRejectsSourceRootSymlinkAndResolvesVaultSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}

	tmp := t.TempDir()
	realRoot := filepath.Join(tmp, "real-vault")
	if err := os.Mkdir(realRoot, 0755); err != nil {
		t.Fatal(err)
	}
	rootLink := filepath.Join(tmp, "vault-link")
	if err := os.Symlink(realRoot, rootLink); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(rootLink)
	if err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	if mgr.RootDir() != resolvedRoot {
		t.Fatalf("RootDir() = %q, want canonical %q", mgr.RootDir(), resolvedRoot)
	}

	realSource := t.TempDir()
	if err := os.WriteFile(filepath.Join(realSource, "file.txt"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	sourceLink := filepath.Join(tmp, "source-link")
	if err := os.Symlink(realSource, sourceLink); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: sourceLink, NoScan: true}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Save(root symlink) error = %v, want clear symbolic-link rejection", err)
	}

	// The real path containing a symlink-configured vault must also be rejected.
	if _, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: tmp, NoScan: true}); err == nil {
		t.Fatal("Save(vault ancestor) succeeded through configured root symlink")
	}
}

func TestRestoreRejectsTargetsInsideVault(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "source")
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("snapshot"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, NoScan: true})
	if err != nil {
		t.Fatal(err)
	}
	contentDir := filepath.Join(st.Dir, "content")

	for name, target := range map[string]string{
		"vault ancestor": tmp,
		"vault root":     mgr.RootDir(),
		"stash dir":      st.Dir,
		"own content":    contentDir,
		"new vault dir":  filepath.Join(mgr.RootDir(), "restore-target"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := mgr.Restore(context.Background(), st.Manifest.ID, target); err == nil {
				t.Fatalf("Restore(target=%q) succeeded, want vault-target rejection", target)
			}
		})
	}

	data, err := os.ReadFile(filepath.Join(contentDir, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "snapshot" {
		t.Fatalf("stash content changed after rejected self-restore: %q", data)
	}

	if runtime.GOOS != "windows" {
		link := filepath.Join(tmp, "content-link")
		if err := os.Symlink(contentDir, link); err != nil {
			t.Fatal(err)
		}
		if _, err := mgr.Restore(context.Background(), st.Manifest.ID, link); err == nil {
			t.Fatal("Restore(symlink to content) succeeded, want canonical vault-target rejection")
		}
	}
}

func TestPrivateVaultDirectoriesAndFileModePreserved(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}

	tmp := t.TempDir()
	root := filepath.Join(tmp, "vault")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	mgr, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(tmp, "secret.txt")
	if err := os.WriteFile(src, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src, 0600); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, Name: "private", NoScan: true})
	if err != nil {
		t.Fatal(err)
	}

	for label, path := range map[string]string{
		"root":    mgr.RootDir(),
		"stash":   st.Dir,
		"content": filepath.Join(st.Dir, "content"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0700 {
			t.Errorf("%s mode = %04o, want 0700", label, got)
		}
	}

	stored := filepath.Join(st.Dir, "content", filepath.Base(src))
	if info, err := os.Stat(stored); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("stored file mode = %04o, want 0600", got)
	}

	target := filepath.Join(tmp, "restored")
	if _, err := mgr.Restore(context.Background(), st.Manifest.ID, target); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(target, filepath.Base(src))); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("restored file mode = %04o, want 0600", got)
	}
}

func TestLongAndUnicodeNamesProduceBoundedUniqueIDs(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "source.txt")
	if err := os.WriteFile(src, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	names := []string{
		strings.Repeat("long-readable-name-", 1000),
		strings.Repeat("資料📦résumé/\u0000", 1000),
	}
	seen := make(map[string]struct{})
	for _, name := range names {
		for range 2 {
			st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, Name: name, NoScan: true})
			if err != nil {
				t.Fatal(err)
			}
			id := st.Manifest.ID
			if len(id) > maxStashIDBytes {
				t.Errorf("ID length = %d, want <= %d: %q", len(id), maxStashIDBytes, id)
			}
			if !validStashID(id) || strings.ContainsAny(id, `/\\`) {
				t.Errorf("unsafe stash ID %q", id)
			}
			if _, exists := seen[id]; exists {
				t.Fatalf("duplicate stash ID %q", id)
			}
			seen[id] = struct{}{}
		}
	}
}

func TestConcurrentSavesUseUniqueExclusiveDirectories(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "source")
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "file.txt"), []byte("concurrent"), 0600); err != nil {
		t.Fatal(err)
	}

	const saves = 32
	type outcome struct {
		stash *Stash
		err   error
	}
	results := make(chan outcome, saves)
	var wg sync.WaitGroup
	for range saves {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st, saveErr := mgr.Save(context.Background(), &SaveOptions{
				SourcePath: src,
				Name:       "same-name",
				NoScan:     true,
			})
			results <- outcome{stash: st, err: saveErr}
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[string]struct{}, saves)
	for result := range results {
		if result.err != nil {
			t.Errorf("concurrent Save: %v", result.err)
			continue
		}
		id := result.stash.Manifest.ID
		if _, exists := seen[id]; exists {
			t.Errorf("duplicate concurrent stash ID %q", id)
			continue
		}
		seen[id] = struct{}{}
		if _, err := mgr.Info(context.Background(), id); err != nil {
			t.Errorf("load concurrent stash %q: %v", id, err)
		}
		if data, err := os.ReadFile(filepath.Join(result.stash.Dir, "content", "file.txt")); err != nil {
			t.Errorf("read concurrent stash %q: %v", id, err)
		} else if string(data) != "concurrent" {
			t.Errorf("concurrent stash %q content = %q", id, data)
		}
	}
	if len(seen) != saves {
		t.Fatalf("unique completed saves = %d, want %d", len(seen), saves)
	}
}

func TestConcurrentManifestMutationsPreserveFields(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "vault")
	mgr, err := NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte(strings.Repeat("payload", 1024)), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: source})
	if err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 2)
	start := make(chan struct{})
	go func() {
		<-start
		_, err := mgr.Compress(context.Background(), st.Manifest.ID, "zstd")
		errs <- err
	}()
	go func() {
		<-start
		errs <- other.SetExpiry(context.Background(), st.Manifest.ID, "1h")
	}()
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}

	got, err := mgr.Info(context.Background(), st.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest.Compression != "zstd" || got.Manifest.CompressedSize == 0 {
		t.Fatalf("compression fields were lost: %+v", got.Manifest)
	}
	if got.Manifest.ExpiresAt == "" {
		t.Fatal("expiry field was lost during concurrent compression")
	}
}
