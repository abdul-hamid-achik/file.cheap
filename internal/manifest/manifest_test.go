package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVideoSummary(t *testing.T) {
	cases := []struct {
		custom map[string]string
		want   string
	}{
		{nil, ""},
		{map[string]string{"source_video": "/x/bug.mp4"}, "/x/bug.mp4"},
		{map[string]string{"source_video": "/x/bug.mp4", "duration_seconds": "124"}, "/x/bug.mp4 (124s)"},
		{map[string]string{"source_video": "/x/bug.mp4", "duration_seconds": "124", "frame_rate": "30"}, "/x/bug.mp4 (124s, 30fps)"},
		{map[string]string{"frame_rate": "30"}, ""}, // no source video -> empty
	}
	for _, c := range cases {
		m := &Manifest{Custom: c.custom}
		if got := m.VideoSummary(); got != c.want {
			t.Errorf("VideoSummary(%v) = %q, want %q", c.custom, got, c.want)
		}
	}
}

func TestScanFiles(t *testing.T) {
	tmp := t.TempDir()
	// Create test files
	files := map[string]string{
		"a.txt":          "hello",
		"sub/b.txt":      "world",
		"sub/deep/c.txt": "!",
	}
	for path, content := range files {
		full := filepath.Join(tmp, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	m := New("test-id", "/source/path")
	if err := m.ScanFiles(tmp); err != nil {
		t.Fatalf("ScanFiles: %v", err)
	}

	if m.FileCount != 3 {
		t.Errorf("FileCount = %d, want 3", m.FileCount)
	}
	if m.TotalSize != 11 { // "hello" + "world" + "!" = 5+5+1
		t.Errorf("TotalSize = %d, want 11", m.TotalSize)
	}
	if m.ContentHash == "" {
		t.Error("ContentHash should not be empty")
	}

	// Check files are sorted
	if m.Files[0].Path != "a.txt" {
		t.Errorf("first file = %s, want a.txt", m.Files[0].Path)
	}
	if m.Files[1].Path != "sub/b.txt" {
		t.Errorf("second file = %s, want sub/b.txt", m.Files[1].Path)
	}
	if m.Files[2].Path != "sub/deep/c.txt" {
		t.Errorf("third file = %s, want sub/deep/c.txt", m.Files[2].Path)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()

	m := New("test-id", "/source")
	m.Tags = []string{"bug", "vidtrace"}
	m.Tool = "vidtrace"
	m.FileCount = 5
	m.TotalSize = 1024

	if err := m.Save(tmp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(tmp)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.ID != "test-id" {
		t.Errorf("ID = %s, want test-id", loaded.ID)
	}
	if loaded.Tool != "vidtrace" {
		t.Errorf("Tool = %s, want vidtrace", loaded.Tool)
	}
	if len(loaded.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(loaded.Tags))
	}
	if !loaded.HasTag("bug") {
		t.Error("should have tag 'bug'")
	}
}

func TestSearchableText(t *testing.T) {
	m := &Manifest{
		Files: []FileEntry{
			{Path: "frames/0001.png"},
			{Path: "ocr/0001.txt"},
			{Path: "transcript/full.txt"},
		},
	}
	text := m.SearchableText()
	if !strings.Contains(text, "frames/0001.png") {
		t.Error("searchable text should contain file paths")
	}
}
