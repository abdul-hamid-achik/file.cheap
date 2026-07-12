//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package stash

import (
	"context"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSaveRejectsFIFOWithoutBlocking(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(filepath.Join(tmp, "vault"))
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(tmp, "source")
	if err := syscall.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(src, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, saveErr := mgr.Save(context.Background(), &SaveOptions{
			SourcePath: src,
			Name:       "fifo",
			NoScan:     true,
		})
		done <- saveErr
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Save with FIFO succeeded, want unsupported-file-type error")
		}
		if !strings.Contains(err.Error(), "unsupported file type") {
			t.Fatalf("Save with FIFO error = %v, want unsupported-file-type error", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Save blocked while reading a FIFO")
	}
}
