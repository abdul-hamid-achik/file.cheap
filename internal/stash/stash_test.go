package stash

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveAndList(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(tmp)
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

func TestRestore(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(tmp)
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
	mgr, err := NewManager(tmp)
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

func TestDrop(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(tmp)
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

func TestMetadataIndexSync(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(tmp)
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
	mgr, err := NewManager(tmp)
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
	mgr, err := NewManager(tmp)
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
	mgr, err := NewManager(tmp)
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
	mgr, err := NewManager(tmp)
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

	for _, badID := range []string{rel, "../../etc", "..", ".", "a/b", ""} {
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
	}

	// The victim must be untouched — no traversal Drop deleted it.
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("victim directory was deleted via traversal: %v", err)
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
