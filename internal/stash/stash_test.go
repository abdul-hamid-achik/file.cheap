package stash

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
	if err := mgr.Restore(context.Background(), st.Manifest.ID, target); err != nil {
		t.Fatalf("Restore: %v", err)
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