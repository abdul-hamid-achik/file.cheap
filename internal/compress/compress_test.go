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
