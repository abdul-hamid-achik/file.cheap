package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

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

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
