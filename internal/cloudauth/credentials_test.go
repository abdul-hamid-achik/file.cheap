package cloudauth

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadAndDropCredentials(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission assertion")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	want := Credentials{ServiceURL: "https://file.cheap", Token: "fcheap_device_" + strings.Repeat("a", 43)}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	path, _ := Path()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("credentials mode = %o, want 600", got)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("credentials = %+v, want %+v", got, want)
	}
	if err := Drop(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("credentials still exist: %v", err)
	}
}

func TestRefreshRotationIsRecoverableAcrossSaveAndLoad(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission assertion")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	initial, err := FromToken("https://file.cheap", Token{
		AccessToken:      "fcheap_device_" + strings.Repeat("a", 43),
		ExpiresIn:        900,
		RefreshExpiresIn: 30 * 24 * 60 * 60,
		RefreshToken:     "fcheap_refresh_" + strings.Repeat("b", 43),
		TokenType:        "Bearer",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := BeginRefresh(initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(pending); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	retried, err := BeginRefresh(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if retried.PendingRefreshToken != pending.PendingRefreshToken || retried.PendingRotationID != pending.PendingRotationID {
		t.Fatal("retry did not preserve the pending refresh rotation")
	}
	completed, err := CompleteRefresh(retried, Token{
		AccessToken:      "fcheap_device_" + strings.Repeat("c", 43),
		ExpiresIn:        900,
		RefreshExpiresIn: 30 * 24 * 60 * 60,
		RefreshToken:     retried.PendingRefreshToken,
		TokenType:        "Bearer",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if completed.RefreshToken != pending.PendingRefreshToken || completed.PendingRefreshToken != "" || completed.PendingRotationID != "" {
		t.Fatal("completed refresh did not promote and clear the pending rotation")
	}
}

func TestSaveRefusesCredentialsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ")
	}
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	err = Save(Credentials{ServiceURL: "https://file.cheap", Token: "fcheap_device_" + strings.Repeat("a", 43)})
	if err == nil {
		t.Fatal("Save succeeded through a symlink")
	}
	data, readErr := os.ReadFile(target)
	if readErr != nil || string(data) != "keep" {
		t.Fatalf("symlink target changed: %q, %v", data, readErr)
	}
}
