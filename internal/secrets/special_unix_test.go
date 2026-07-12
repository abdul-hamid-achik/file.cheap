//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package secrets

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestScanSkipsFIFOAndSymlinkToFIFOWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "pipe.txt")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	externalFIFO := filepath.Join(t.TempDir(), "external-pipe")
	if err := syscall.Mkfifo(externalFIFO, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalFIFO, filepath.Join(dir, "linked-pipe.txt")); err != nil {
		t.Fatal(err)
	}

	done := make(chan []Finding, 1)
	go func() { done <- Scan(dir) }()
	select {
	case findings := <-done:
		if len(findings) != 0 {
			t.Fatalf("Scan returned findings for special files: %+v", findings)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Scan blocked opening a FIFO or symlink target")
	}
}
