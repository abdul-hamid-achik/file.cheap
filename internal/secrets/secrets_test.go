package secrets

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestScanContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ScanContext(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("ScanContext error = %v, want context.Canceled", err)
	}
}

func TestScanDetectsSecrets(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "config.env", "AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE\napi_key = \"abcd1234efgh5678ijkl\"\n")
	mustWrite(t, dir, "id_rsa", "-----BEGIN OPENSSH PRIVATE KEY-----\nbase64stuff\n")
	mustWrite(t, dir, "clean.txt", "just some harmless prose about the weather\n")

	findings := Scan(dir)
	if len(findings) == 0 {
		t.Fatal("expected secret findings, got none")
	}

	rules := map[string]bool{}
	files := map[string]bool{}
	for _, f := range findings {
		rules[f.Rule] = true
		files[f.File] = true
		if f.Line <= 0 {
			t.Errorf("finding has invalid line: %+v", f)
		}
	}
	if !rules["aws-access-key"] {
		t.Error("expected aws-access-key rule to match")
	}
	if !rules["private-key"] {
		t.Error("expected private-key rule to match")
	}
	if files["clean.txt"] {
		t.Error("clean.txt should not produce findings")
	}
}

func TestRulesDistinct(t *testing.T) {
	findings := []Finding{
		{File: "a", Rule: "aws-access-key", Line: 1},
		{File: "b", Rule: "aws-access-key", Line: 2},
		{File: "c", Rule: "private-key", Line: 3},
	}
	got := Rules(findings)
	if len(got) != 2 {
		t.Errorf("Rules() = %v, want 2 distinct", got)
	}
}

func TestScanDoesNotFollowExternalSecretSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "external.env")
	if err := os.WriteFile(outside, []byte("token=abcdefghijklmnop123456"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.env")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	findings := Scan(dir)
	if len(findings) != 0 {
		t.Fatalf("Scan followed external secret symlink: %+v", findings)
	}

	rootLink := filepath.Join(t.TempDir(), "linked-root")
	if err := os.Symlink(filepath.Dir(outside), rootLink); err != nil {
		t.Skipf("root symlink unavailable: %v", err)
	}
	if findings := Scan(rootLink); len(findings) != 0 {
		t.Fatalf("Scan followed a symlink root: %+v", findings)
	}
}

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestScanReportAccountsForSkippedFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "config.env", "aws_id = AKIAIOSFODNN7EXAMPLE\n")
	mustWrite(t, dir, "clean.txt", "harmless\n")
	// There is no per-file size cap: a secret past 1 MiB is still found.
	mustWrite(t, dir, "trace.bin", strings.Repeat("x", 1<<20)+"\nAKIAIOSFODNN7EXAMPLE\n")
	if err := os.Symlink(filepath.Join(dir, "clean.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	report, err := ScanReportContext(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, f := range report.Findings {
		got[f.File] = f.Line
	}
	if got["config.env"] != 1 || got["trace.bin"] != 2 || len(got) != 2 {
		t.Fatalf("findings = %+v, want config.env:1 and trace.bin:2", report.Findings)
	}
	if report.FilesScanned != 3 || report.FilesSkipped != 1 {
		t.Fatalf("scanned/skipped = %d/%d, want 3/1", report.FilesScanned, report.FilesSkipped)
	}
	if report.SkippedReasons[SkipNotRegular] != 1 || len(report.SkippedReasons) != 1 {
		t.Fatalf("skipped reasons = %v", report.SkippedReasons)
	}
}

func TestScanContextMatchesScanReportFindings(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "config.env", "AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE\n")
	findings, err := ScanContext(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	report, err := ScanReportContext(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) == 0 || !slices.Equal(findings, report.Findings) {
		t.Fatalf("ScanContext %+v diverged from ScanReportContext %+v", findings, report.Findings)
	}
}
