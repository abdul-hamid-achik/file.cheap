package stash

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
)

// TestParseTTL verifies TTL duration parsing mirrors ParseSince but for
// future expiry.
func TestParseTTL(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{in: "24h", want: 24 * time.Hour, ok: true},
		{in: "90m", want: 90 * time.Minute, ok: true},
		{in: "7d", want: 7 * 24 * time.Hour, ok: true},
		{in: "2w", want: 14 * 24 * time.Hour, ok: true},
		{in: "garbage", ok: false},
		{in: "5x", ok: false},
	}
	for _, c := range cases {
		got, err := ParseTTL(c.in)
		if !c.ok {
			if err == nil {
				t.Errorf("ParseTTL(%q) = no error, want error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTTL(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseTTL(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestSaveWithTTL verifies that a stash saved with --ttl gets an expires_at
// in the manifest, and that it shows up as expired after the TTL elapses.
func TestSaveWithTTL(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "ttl-test",
		TTL:        "1s",
	})
	if err != nil {
		t.Fatalf("Save with TTL: %v", err)
	}

	if st.Manifest.ExpiresAt == "" {
		t.Fatal("ExpiresAt should not be empty when TTL is set")
	}

	// Not expired immediately.
	if IsExpired(st.Manifest) {
		t.Error("stash should not be expired immediately after save")
	}

	// Wait for expiry.
	time.Sleep(2 * time.Second)
	if !IsExpired(st.Manifest) {
		t.Error("stash should be expired after TTL duration")
	}
}

// TestSaveWithoutTTL verifies that a stash saved without TTL has no expires_at
// and never reports as expired.
func TestSaveWithoutTTL(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "no-ttl-test",
	})
	if err != nil {
		t.Fatalf("Save without TTL: %v", err)
	}

	if st.Manifest.ExpiresAt != "" {
		t.Errorf("ExpiresAt should be empty without TTL, got %q", st.Manifest.ExpiresAt)
	}
	if IsExpired(st.Manifest) {
		t.Error("stash without TTL should never be expired")
	}
}

// TestListHidesExpired verifies that List excludes expired stashes by default,
// and --include-expired shows them.
func TestListHidesExpired(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Save a stash with a 1-second TTL.
	if _, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "expiring",
		TTL:        "1s",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Before expiry: list shows it.
	if got, _ := mgr.List(context.Background(), ""); len(got) != 1 {
		t.Fatalf("List before expiry = %d, want 1", len(got))
	}

	// Wait for expiry.
	time.Sleep(2 * time.Second)

	// After expiry: default List hides it.
	if got, _ := mgr.List(context.Background(), ""); len(got) != 0 {
		t.Errorf("List after expiry (default) = %d, want 0", len(got))
	}

	// With IncludeExpired: list shows it.
	if got, _ := mgr.ListFiltered(context.Background(), ListOptions{IncludeExpired: true}); len(got) != 1 {
		t.Errorf("List after expiry (IncludeExpired) = %d, want 1", len(got))
	}
}

// TestSweepExpired verifies that SweepExpired reports expired stashes in
// dry-run mode and actually drops them when applied.
func TestSweepExpired(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Stash with a 1s TTL.
	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "expiring",
		TTL:        "1s",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Dry-run: reports expired but doesn't drop.
	res, err := mgr.SweepExpired(context.Background(), false, "", nil)
	if err != nil {
		t.Fatalf("Sweep dry-run: %v", err)
	}
	if res.Applied {
		t.Error("dry-run should not have Applied=true")
	}
	if len(res.Expired) != 1 || res.Expired[0] != st.Manifest.ID {
		t.Errorf("Expired = %v, want [%s]", res.Expired, st.Manifest.ID)
	}
	if !mgr.Exists(st.Manifest.ID) {
		t.Error("stash should still exist after dry-run sweep")
	}

	// Apply: actually drops.
	dropped := []string{}
	res, err = mgr.SweepExpired(context.Background(), true, "", func(id string) error {
		dropped = append(dropped, id)
		return nil
	})
	if err != nil {
		t.Fatalf("Sweep apply: %v", err)
	}
	if !res.Applied {
		t.Error("apply should have Applied=true")
	}
	if len(res.Expired) != 1 {
		t.Errorf("Expired = %d, want 1", len(res.Expired))
	}
	if mgr.Exists(st.Manifest.ID) {
		t.Error("stash should not exist after sweep --apply")
	}
	if len(dropped) != 1 || dropped[0] != st.Manifest.ID {
		t.Errorf("dropIndex called with %v, want [%s]", dropped, st.Manifest.ID)
	}
}

// TestSweepKeepTag verifies that a stash with the keep tag is not swept even
// when expired.
func TestSweepKeepTag(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "keep-me",
		TTL:        "1s",
		Tags:       []string{"keep"},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	time.Sleep(2 * time.Second)

	// Sweep with keepTag="keep": should NOT drop.
	res, err := mgr.SweepExpired(context.Background(), true, "keep", nil)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(res.Expired) != 0 {
		t.Errorf("Expired with keep tag = %v, want empty", res.Expired)
	}
	if !mgr.Exists(st.Manifest.ID) {
		t.Error("stash with keep tag should not be swept")
	}

	// Sweep without keepTag: should drop.
	res, err = mgr.SweepExpired(context.Background(), true, "", nil)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(res.Expired) != 1 {
		t.Errorf("Expired without keep tag = %d, want 1", len(res.Expired))
	}
	if mgr.Exists(st.Manifest.ID) {
		t.Error("stash without keep tag should be swept")
	}
}

// TestSetExpiry verifies that SetExpiry can set and clear the TTL on an
// existing stash.
func TestSetExpiry(t *testing.T) {
	tmp := t.TempDir()
	mgr, err := NewManager(tmp)
	if err != nil {
		t.Fatal(err)
	}

	srcDir := filepath.Join(tmp, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	st, err := mgr.Save(context.Background(), &SaveOptions{
		SourcePath: srcDir,
		Name:       "set-ttl-test",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Initially no expiry.
	info, _ := mgr.Info(context.Background(), st.Manifest.ID)
	if info.Manifest.ExpiresAt != "" {
		t.Errorf("ExpiresAt should be empty initially, got %q", info.Manifest.ExpiresAt)
	}

	// Set a 7-day TTL.
	if err := mgr.SetExpiry(context.Background(), st.Manifest.ID, "7d"); err != nil {
		t.Fatalf("SetExpiry: %v", err)
	}
	info, _ = mgr.Info(context.Background(), st.Manifest.ID)
	if info.Manifest.ExpiresAt == "" {
		t.Fatal("ExpiresAt should be set after SetExpiry")
	}
	if IsExpired(info.Manifest) {
		t.Error("stash should not be expired with a 7d TTL")
	}

	// Clear the TTL.
	if err := mgr.SetExpiry(context.Background(), st.Manifest.ID, ""); err != nil {
		t.Fatalf("SetExpiry clear: %v", err)
	}
	info, _ = mgr.Info(context.Background(), st.Manifest.ID)
	if info.Manifest.ExpiresAt != "" {
		t.Errorf("ExpiresAt should be empty after clearing, got %q", info.Manifest.ExpiresAt)
	}
}

// TestManifestExpiresAtRoundTrip verifies the ExpiresAt field survives JSON
// serialization/deserialization.
func TestManifestExpiresAtRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := manifest.New("test_123", "/src")
	m.ExpiresAt = "2026-12-31T23:59:59Z"
	if err := m.Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := manifest.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ExpiresAt != "2026-12-31T23:59:59Z" {
		t.Errorf("ExpiresAt = %q, want 2026-12-31T23:59:59Z", loaded.ExpiresAt)
	}
}
