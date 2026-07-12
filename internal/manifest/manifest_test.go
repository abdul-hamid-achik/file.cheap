package manifest

import (
	"context"
	"errors"
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

func TestScanFilesContextHonorsCancellation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := New("cancelled", dir).ScanFilesContext(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatalf("ScanFilesContext error = %v, want context.Canceled", err)
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

func TestLoadRejectsUnsupportedSchema(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"schema_version":"99.0","id":"future","created_at":"2026-01-01T00:00:00Z"}`)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "unsupported manifest schema") {
		t.Fatalf("Load error = %v, want unsupported schema", err)
	}
}

func TestLoadAdoptsLegacyManifestWithoutSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	data := []byte(`{"id":"legacy","created_at":"2026-01-01T00:00:00Z"}`)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	man, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if man.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", man.SchemaVersion, SchemaVersion)
	}
}

func TestLoadRejectsUnsafeMetadata(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "bad expiry",
			body: `{"id":"stash","created_at":"2026-01-01T00:00:00Z","expires_at":"tomorrow"}`,
			want: "invalid manifest expires_at",
		},
		{
			name: "absolute file path",
			body: `{"id":"stash","created_at":"2026-01-01T00:00:00Z","files":[{"path":"/etc/passwd","size":1}]}`,
			want: "invalid manifest file path",
		},
		{
			name: "traversal file path",
			body: `{"id":"stash","created_at":"2026-01-01T00:00:00Z","files":[{"path":"../outside","size":1}]}`,
			want: "invalid manifest file path",
		},
		{
			name: "duplicate file path",
			body: `{"id":"stash","created_at":"2026-01-01T00:00:00Z","files":[{"path":"a.txt","size":1},{"path":"a.txt","size":1}]}`,
			want: "duplicate manifest file path",
		},
		{
			name: "negative file size",
			body: `{"id":"stash","created_at":"2026-01-01T00:00:00Z","files":[{"path":"a.txt","size":-1}]}`,
			want: "invalid negative size",
		},
		{
			name: "file count mismatch",
			body: `{"id":"stash","created_at":"2026-01-01T00:00:00Z","file_count":0,"total_size":1,"files":[{"path":"a.txt","size":1}]}`,
			want: "file_count",
		},
		{
			name: "total size mismatch",
			body: `{"id":"stash","created_at":"2026-01-01T00:00:00Z","file_count":1,"total_size":2,"files":[{"path":"a.txt","size":1}]}`,
			want: "total_size",
		},
		{
			name: "invalid file hash",
			body: `{"id":"stash","created_at":"2026-01-01T00:00:00Z","file_count":1,"total_size":1,"files":[{"path":"a.txt","size":1,"hash":"nope"}]}`,
			want: "invalid SHA-256",
		},
		{
			name: "invalid content hash",
			body: `{"id":"stash","created_at":"2026-01-01T00:00:00Z","content_hash":"nope"}`,
			want: "invalid manifest content_hash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(tt.body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsOversizedManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, maxManifestBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("Load error = %v, want size limit", err)
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
