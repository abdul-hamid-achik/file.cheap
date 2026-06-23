package diff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
)

func makeStash(t *testing.T, files map[string]string) string {
	t.Helper()
	tmp := t.TempDir()
	stashDir := filepath.Join(tmp, "stash")
	content := filepath.Join(stashDir, "content")
	if err := os.MkdirAll(content, 0755); err != nil {
		t.Fatal(err)
	}
	for rel, c := range files {
		p := filepath.Join(content, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		if err := os.WriteFile(p, []byte(c), 0644); err != nil {
			t.Fatal(err)
		}
	}
	man := manifest.New("test", content)
	if err := man.ScanFiles(content); err != nil {
		t.Fatal(err)
	}
	if err := man.Save(stashDir); err != nil {
		t.Fatal(err)
	}
	return stashDir
}

func TestCompareStashToDir(t *testing.T) {
	stashDir := makeStash(t, map[string]string{
		"same.txt":    "unchanged",
		"changed.txt": "original content",
		"removed.txt": "only in stash",
	})

	target := t.TempDir()
	write(t, target, "same.txt", "unchanged")
	write(t, target, "changed.txt", "different content entirely")
	write(t, target, "added.txt", "only in target")

	res, err := CompareStashToDir(stashDir, target)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.OnlyInStash, "removed.txt") {
		t.Errorf("OnlyInStash = %v, want removed.txt", res.OnlyInStash)
	}
	if !contains(res.OnlyInTarget, "added.txt") {
		t.Errorf("OnlyInTarget = %v, want added.txt", res.OnlyInTarget)
	}
	if len(res.Changed) != 1 || res.Changed[0].Path != "changed.txt" {
		t.Errorf("Changed = %+v, want [changed.txt]", res.Changed)
	}
	if res.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", res.Unchanged)
	}
}

// TestCompareDetectsSameSizeChange guards the regression where diff compared by
// size only and missed a same-length content edit.
func TestCompareDetectsSameSizeChange(t *testing.T) {
	stashDir := makeStash(t, map[string]string{"f.txt": "AAAAA"})
	target := t.TempDir()
	write(t, target, "f.txt", "BBBBB") // same length, different content

	res, err := CompareStashToDir(stashDir, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Changed) != 1 {
		t.Fatalf("Changed = %d, want 1 (same-size content change must be detected)", len(res.Changed))
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
