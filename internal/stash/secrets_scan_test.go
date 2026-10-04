package stash

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeScanFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	src := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return src
}

func vaultEntries(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestSaveFailOnSecretsLeavesNoStashDirOrIndexRow(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	mgr, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	src := writeScanFixture(t, map[string]string{
		"a.env":     "aws_id = AKIAIOSFODNN7EXAMPLE\n",
		"clean.txt": "hello\n",
	})
	before := vaultEntries(t, root)

	_, err = mgr.Save(context.Background(), &SaveOptions{SourcePath: src, FailOnSecrets: true})
	var found *SecretsFoundError
	if !errors.As(err, &found) {
		t.Fatalf("Save error = %v, want *SecretsFoundError", err)
	}
	if len(found.Findings) != 1 || found.Findings[0].File != "a.env" || found.Findings[0].Rule != "aws-access-key" || found.Findings[0].Line != 1 {
		t.Fatalf("findings = %+v", found.Findings)
	}
	if found.Scan.FilesScanned != 2 {
		t.Fatalf("scan = %+v", found.Scan)
	}
	if strings.Contains(err.Error(), "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("error leaks the secret value: %v", err)
	}
	if after := vaultEntries(t, root); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Fatalf("vault changed after a refused save: before=%v after=%v", before, after)
	}
	stashes, err := mgr.List(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stashes) != 0 {
		t.Fatalf("refused save left %d stash(es)", len(stashes))
	}
}

func TestSaveFailOnSecretsPassesWhenClean(t *testing.T) {
	mgr, err := NewManager(filepath.Join(t.TempDir(), "vault"))
	if err != nil {
		t.Fatal(err)
	}
	src := writeScanFixture(t, map[string]string{"clean.txt": "hello\n"})
	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, FailOnSecrets: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Secrets) != 0 || st.Manifest.ID == "" {
		t.Fatalf("stash = %+v", st)
	}
}

func TestSaveFailOnSecretsRejectsNoScan(t *testing.T) {
	root := filepath.Join(t.TempDir(), "vault")
	mgr, err := NewManager(root)
	if err != nil {
		t.Fatal(err)
	}
	src := writeScanFixture(t, map[string]string{"clean.txt": "hello\n"})
	if _, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, FailOnSecrets: true, NoScan: true}); err == nil {
		t.Fatal("FailOnSecrets with NoScan must be rejected")
	}
	if stashes, _ := mgr.List(context.Background(), ""); len(stashes) != 0 {
		t.Fatalf("rejected save left %d stash(es)", len(stashes))
	}
}

func TestSavePersistsScanAccountingInManifest(t *testing.T) {
	mgr, err := NewManager(filepath.Join(t.TempDir(), "vault"))
	if err != nil {
		t.Fatal(err)
	}
	src := writeScanFixture(t, map[string]string{
		"a.txt": "alpha\n",
		"b.txt": strings.Repeat("b", 400) + "\n",
		"c.txt": strings.Repeat("c", 400) + "\n",
	})
	st, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, SecretScanBudget: 500})
	if err != nil {
		t.Fatal(err)
	}
	custom := st.Manifest.Custom
	if custom["secrets_files_scanned"] != "2" || custom["secrets_files_skipped"] != "1" {
		t.Fatalf("custom = %v, want scanned=2 skipped=1", custom)
	}
	if st.SecretScan.SkippedReasons["budget"] != 1 {
		t.Fatalf("skipped reasons = %v", st.SecretScan.SkippedReasons)
	}
	// A save with the scan disabled records no accounting (nothing was scanned).
	off, err := mgr.Save(context.Background(), &SaveOptions{SourcePath: src, NoScan: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := off.Manifest.Custom["secrets_files_scanned"]; ok {
		t.Fatalf("NoScan save recorded accounting: %v", off.Manifest.Custom)
	}
	// The accounting survives a reload from disk.
	loaded, err := mgr.Info(context.Background(), st.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Manifest.Custom["secrets_files_scanned"] != "2" {
		t.Fatalf("reloaded custom = %v", loaded.Manifest.Custom)
	}
}

func TestScanAccountingKeysAreReservedMetadata(t *testing.T) {
	for _, key := range []string{"secrets_files_scanned", "secrets_files_skipped"} {
		if !IsReservedMetadataKey(key) {
			t.Errorf("%s must be reserved", key)
		}
		if _, err := ParseMetadata([]string{key + "=999"}); err == nil {
			t.Errorf("--meta %s must be refused", key)
		}
	}
}
