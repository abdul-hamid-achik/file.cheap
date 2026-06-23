package compress

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveExtractRoundTrip(t *testing.T) {
	// Extract detects the format by extension, so each algorithm uses its own.
	ext := map[Algorithm]string{Zstd: "out.tar.zst", Gzip: "out.tar.gz", None: "out.tar"}
	for _, algo := range []Algorithm{Zstd, Gzip, None} {
		t.Run(string(algo), func(t *testing.T) {
			tmp := t.TempDir()
			src := filepath.Join(tmp, "src")
			if err := os.MkdirAll(filepath.Join(src, "sub"), 0755); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{
				"a.txt":     "hello world",
				"sub/b.txt": "nested content here",
			}
			for rel, content := range files {
				if err := os.WriteFile(filepath.Join(src, rel), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			archive := filepath.Join(tmp, ext[algo])
			if _, err := Archive(src, archive, algo); err != nil {
				t.Fatalf("Archive(%s): %v", algo, err)
			}
			if fi, err := os.Stat(archive); err != nil || fi.Size() == 0 {
				t.Fatalf("archive not created: %v", err)
			}

			dst := filepath.Join(tmp, "dst")
			if err := Extract(archive, dst); err != nil {
				t.Fatalf("Extract(%s): %v", algo, err)
			}
			for rel, want := range files {
				got, err := os.ReadFile(filepath.Join(dst, rel))
				if err != nil {
					t.Fatalf("read %s: %v", rel, err)
				}
				if string(got) != want {
					t.Errorf("%s = %q, want %q", rel, got, want)
				}
			}
		})
	}
}

func TestArchiveUnknownAlgo(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	_ = os.MkdirAll(src, 0755)
	if _, err := Archive(src, filepath.Join(tmp, "x"), Algorithm("bogus")); err == nil {
		t.Error("expected error for unknown algorithm")
	}
}

func TestExtractRejectsTraversal(t *testing.T) {
	// isSafePath must reject paths that escape the target directory.
	if isSafePath("/tmp/target", "/tmp/target/../evil") {
		t.Error("traversal path should be rejected")
	}
	if !isSafePath("/tmp/target", "/tmp/target/ok/file.txt") {
		t.Error("safe path should be accepted")
	}
}

// TestArchiveExtractSymlinks verifies symlinks survive an archive/extract round
// trip: safe relative links (incl. dangling) are preserved, while links that
// would escape the extraction root are dropped (not fatal). Regression test for
// the "Archive crashes on symlinks" and "Extract silently drops symlinks" bugs.
func TestArchiveExtractSymlinks(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "real.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, "real.txt", filepath.Join(src, "link.txt"))            // safe, valid
	mustSymlink(t, "nonexistent.txt", filepath.Join(src, "dangling.txt")) // safe, dangling
	mustSymlink(t, "/etc/passwd", filepath.Join(src, "evil.txt"))         // unsafe (absolute)

	arc := filepath.Join(t.TempDir(), "out.tar.zst")
	if _, err := Archive(src, arc, Zstd); err != nil {
		t.Fatalf("Archive must not fail on symlinks: %v", err)
	}
	dst := t.TempDir()
	if err := Extract(arc, dst); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if got, err := os.Readlink(filepath.Join(dst, "link.txt")); err != nil || got != "real.txt" {
		t.Errorf("link.txt = %q (err %v); want real.txt", got, err)
	}
	if got, err := os.Readlink(filepath.Join(dst, "dangling.txt")); err != nil || got != "nonexistent.txt" {
		t.Errorf("dangling.txt = %q (err %v); want nonexistent.txt", got, err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "evil.txt")); !os.IsNotExist(err) {
		t.Errorf("unsafe absolute symlink should be dropped on extract, got err=%v", err)
	}
}

func mustSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Fatalf("symlink %s -> %s: %v", newname, oldname, err)
	}
}

// TestExtractDoesNotWriteThroughPlantedSymlink verifies that a pre-existing
// symlink at a restore destination is replaced (not followed), so extraction
// cannot clobber a file outside the target. Regression test for the medium
// symlink-escape (no O_NOFOLLOW) finding.
func TestExtractDoesNotWriteThroughPlantedSymlink(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "config.json"), []byte("real"), 0644); err != nil {
		t.Fatal(err)
	}
	arc := filepath.Join(t.TempDir(), "a.tar.zst")
	if _, err := Archive(src, arc, Zstd); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	outside := t.TempDir()
	victim := filepath.Join(outside, "victim.txt")
	if err := os.WriteFile(victim, []byte("ORIGINAL"), 0644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, victim, filepath.Join(target, "config.json")) // plant before extract

	if err := Extract(arc, target); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if b, _ := os.ReadFile(victim); string(b) != "ORIGINAL" {
		t.Errorf("victim outside target was overwritten through a planted symlink: %q", b)
	}
	fi, err := os.Lstat(filepath.Join(target, "config.json"))
	if err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("config.json should be a real file, not a symlink (err %v)", err)
	}
}
