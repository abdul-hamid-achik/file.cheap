//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package detect

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestDetectSkipsFIFOAndSymlinkToFIFOWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"metadata.json", "notes.txt"} {
		if err := syscall.Mkfifo(filepath.Join(dir, name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "timeline.json"), []byte(`{"entries":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	externalFIFO := filepath.Join(t.TempDir(), "external-pipe")
	if err := syscall.Mkfifo(externalFIFO, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalFIFO, filepath.Join(dir, "linked.txt")); err != nil {
		t.Fatal(err)
	}

	done := make(chan Result, 1)
	go func() { done <- Detect(dir) }()
	select {
	case result := <-done:
		if result.Type != TypeGeneric {
			t.Fatalf("Detect(FIFO bundle) type = %q, want generic", result.Type)
		}
		if len(result.SearchableFiles) != 1 || result.SearchableFiles[0] != "timeline.json" {
			t.Fatalf("Detect collected special files: %+v", result.SearchableFiles)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Detect blocked opening a FIFO or symlink target")
	}

	if _, ok := VidtraceMetadata(dir); ok {
		t.Fatal("VidtraceMetadata accepted FIFO metadata.json")
	}
}
