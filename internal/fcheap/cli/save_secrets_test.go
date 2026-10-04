package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
)

const fakeAWSKey = "AKIAIOSFODNN7EXAMPLE"

type saveRun struct {
	stdout, stderr string
	decoded        map[string]any
	err            error
	exitCode       int // -1 when exitProcess was not called
	vault          string
}

// runSave runs `fcheap save` in-process against a temp vault. setup adjusts
// the flag variables before the run; they are restored afterwards.
func runSave(t *testing.T, source string, json bool, setup func()) saveRun {
	t.Helper()
	oldCfg, oldPrinter, oldRootCtx := cfg, printer, rootCtx
	oldTags, oldTool, oldSource, oldMeta := saveTags, saveTool, saveSource, saveMeta
	oldNoScan, oldNoCompress, oldIndex := saveNoScan, saveNoCompress, saveIndex
	oldFail, oldBudget, oldExit := saveFailOnSecrets, saveScanBudgetMiB, exitProcess
	t.Cleanup(func() {
		cfg, printer, rootCtx = oldCfg, oldPrinter, oldRootCtx
		saveTags, saveTool, saveSource, saveMeta = oldTags, oldTool, oldSource, oldMeta
		saveNoScan, saveNoCompress, saveIndex = oldNoScan, oldNoCompress, oldIndex
		saveFailOnSecrets, saveScanBudgetMiB, exitProcess = oldFail, oldBudget, oldExit
	})
	var stdout, stderr bytes.Buffer
	run := saveRun{exitCode: -1, vault: filepath.Join(t.TempDir(), "vault")}
	cfg = &config.Config{StashDir: run.vault, Compression: "zstd"}
	printer = output.New(output.WithJSON(json), output.WithOutput(&stdout), output.WithErrOutput(&stderr), output.WithNoColor(true))
	rootCtx = context.Background()
	saveTags, saveTool, saveSource, saveMeta = nil, "", "", nil
	saveNoScan, saveNoCompress, saveIndex = false, true, false
	saveFailOnSecrets, saveScanBudgetMiB = false, 0
	exitProcess = func(code int) { run.exitCode = code }
	if setup != nil {
		setup()
	}
	run.err = saveCmd.RunE(saveCmd, []string{source})
	run.stdout, run.stderr = stdout.String(), stderr.String()
	if json && run.stdout != "" {
		if err := decodeJSON(run.stdout, &run.decoded); err != nil {
			t.Fatalf("decode save %q: %v", run.stdout, err)
		}
	}
	return run
}

func decodeJSON(raw string, into *map[string]any) error {
	return json.Unmarshal([]byte(raw), into)
}

func runSaveJSON(t *testing.T, source string, noScan bool) (string, map[string]any) {
	t.Helper()
	run := runSave(t, source, true, func() { saveNoScan = noScan })
	if run.err != nil {
		t.Fatalf("save: %v", run.err)
	}
	return run.stdout, run.decoded
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func vaultStashDirs(t *testing.T, vault string) []string {
	t.Helper()
	entries, err := os.ReadDir(vault)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs
}

func TestSaveJSONReportsSecretScanCoverage(t *testing.T) {
	source := t.TempDir()
	writeFiles(t, source, map[string]string{
		"config.env": "AWS_ACCESS_KEY=" + fakeAWSKey + "\n",
		"clean.txt":  "harmless\n",
		"trace.bin":  strings.Repeat("x", 3<<20),
	})

	// A 1 MiB scan budget cannot cover the 3 MiB trace: it is counted, not hidden.
	run := runSave(t, source, true, func() { saveScanBudgetMiB = 1 })
	if run.err != nil {
		t.Fatalf("save: %v", run.err)
	}
	raw, decoded := run.stdout, run.decoded
	if strings.Contains(raw, fakeAWSKey) {
		t.Fatal("save --json must never carry a secret value")
	}
	secrets, ok := decoded["secrets"].(map[string]any)
	if !ok {
		t.Fatalf("missing secrets object in %s", raw)
	}
	if secrets["enabled"] != true || secrets["found"].(float64) < 1 {
		t.Fatalf("secrets = %v", secrets)
	}
	if secrets["files_scanned"].(float64) != 2 || secrets["files_skipped"].(float64) != 1 {
		t.Fatalf("scanned/skipped = %v/%v, want 2/1", secrets["files_scanned"], secrets["files_skipped"])
	}
	if reasons := secrets["skipped_reasons"].(map[string]any); reasons["budget"].(float64) != 1 {
		t.Fatalf("skipped_reasons = %v", reasons)
	}
	rules := secrets["rules"].([]any)
	if len(rules) == 0 || rules[0] != "aws-access-key" {
		t.Fatalf("rules = %v", rules)
	}
	findings, ok := secrets["findings"].([]any)
	if !ok || len(findings) == 0 {
		t.Fatalf("findings = %v", secrets["findings"])
	}
	first := findings[0].(map[string]any)
	if first["file"] != "config.env" || first["line"].(float64) != 1 || len(first) != 3 {
		t.Fatalf("finding = %v, want exactly file/rule/line", first)
	}
	if secrets["findings_truncated"] != false {
		t.Fatalf("findings_truncated = %v", secrets["findings_truncated"])
	}
	for key := range secrets {
		switch key {
		case "enabled", "found", "rules", "files_scanned", "files_skipped", "skipped_reasons", "findings", "findings_truncated":
		default:
			t.Fatalf("unexpected secrets key %q (values and line content must never be serialized)", key)
		}
	}
	// Existing consumers keep reading the manifest custom fields, which now also
	// carry the scan accounting.
	custom := decoded["custom"].(map[string]any)
	if custom["secrets_found"] == nil || !strings.Contains(custom["secrets_rules"].(string), "aws-access-key") {
		t.Fatalf("custom secrets fields changed: %v", custom)
	}
	if custom["secrets_files_scanned"] != "2" || custom["secrets_files_skipped"] != "1" {
		t.Fatalf("custom scan accounting = %v", custom)
	}
}

func TestSaveJSONSecretsIsExplicitForCleanAndDisabledScans(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "clean.txt"), []byte("harmless\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, decoded := runSaveJSON(t, source, false)
	clean := decoded["secrets"].(map[string]any)
	if clean["enabled"] != true || clean["found"].(float64) != 0 || clean["files_scanned"].(float64) != 1 || clean["files_skipped"].(float64) != 0 {
		t.Fatalf("clean scan = %v", clean)
	}
	if rules, ok := clean["rules"].([]any); !ok || len(rules) != 0 {
		t.Fatalf("rules must be an empty array, got %v", clean["rules"])
	}
	if findings, ok := clean["findings"].([]any); !ok || len(findings) != 0 {
		t.Fatalf("findings must be an empty array, got %v", clean["findings"])
	}
	custom, _ := decoded["custom"].(map[string]any)
	if _, present := custom["secrets_found"]; present {
		t.Fatal("clean scans must not add custom.secrets_found")
	}

	_, decoded = runSaveJSON(t, source, true)
	disabled := decoded["secrets"].(map[string]any)
	if disabled["enabled"] != false || disabled["files_scanned"].(float64) != 0 {
		t.Fatalf("disabled scan must be distinguishable from a clean one: %v", disabled)
	}
}

func TestSaveFailOnSecretsRefusesWithDistinctExitCodeAndNoStash(t *testing.T) {
	source := t.TempDir()
	writeFiles(t, source, map[string]string{
		"config.env": "aws_id = " + fakeAWSKey + "\n",
		"clean.txt":  "harmless\n",
	})
	run := runSave(t, source, false, func() { saveFailOnSecrets = true })
	if run.exitCode != ExitSecretsFound || ExitSecretsFound != 4 {
		t.Fatalf("exit code = %d, want %d", run.exitCode, ExitSecretsFound)
	}
	if run.err == nil {
		t.Fatal("a refused save must return an error")
	}
	if dirs := vaultStashDirs(t, run.vault); len(dirs) != 0 {
		t.Fatalf("refused save left stash dirs: %v", dirs)
	}
	for name, text := range map[string]string{"stdout": run.stdout, "stderr": run.stderr, "error": run.err.Error()} {
		if strings.Contains(text, fakeAWSKey) {
			t.Fatalf("%s leaks the secret value: %q", name, text)
		}
	}
	if !strings.Contains(run.stderr, "config.env:1 [aws-access-key]") || !strings.Contains(run.stderr, "nothing was saved") {
		t.Fatalf("stderr = %q, want file:line [rule] and the refusal", run.stderr)
	}
}

func TestSaveFailOnSecretsJSONEmitsSecretsObject(t *testing.T) {
	source := t.TempDir()
	writeFiles(t, source, map[string]string{"config.env": "aws_id = " + fakeAWSKey + "\n"})
	run := runSave(t, source, true, func() { saveFailOnSecrets = true })
	if run.exitCode != 4 {
		t.Fatalf("exit code = %d", run.exitCode)
	}
	if strings.Contains(run.stdout, fakeAWSKey) {
		t.Fatal("JSON must never carry a secret value")
	}
	if run.decoded["status"] != "blocked_secrets_found" || run.decoded["saved"] != false || run.decoded["exit_code"].(float64) != 4 {
		t.Fatalf("blocked output = %v", run.decoded)
	}
	if _, hasManifest := run.decoded["manifest"]; hasManifest {
		t.Fatal("a refused save has no manifest")
	}
	secrets := run.decoded["secrets"].(map[string]any)
	findings := secrets["findings"].([]any)
	if secrets["enabled"] != true || secrets["found"].(float64) != 1 || len(findings) != 1 {
		t.Fatalf("secrets = %v", secrets)
	}
	if f := findings[0].(map[string]any); f["file"] != "config.env" || f["rule"] != "aws-access-key" || f["line"].(float64) != 1 {
		t.Fatalf("finding = %v", f)
	}
	if dirs := vaultStashDirs(t, run.vault); len(dirs) != 0 {
		t.Fatalf("refused save left stash dirs: %v", dirs)
	}
}

func TestSaveFailOnSecretsSavesCleanContentNormally(t *testing.T) {
	source := t.TempDir()
	writeFiles(t, source, map[string]string{"clean.txt": "harmless\n"})
	run := runSave(t, source, true, func() { saveFailOnSecrets = true })
	if run.err != nil || run.exitCode != -1 {
		t.Fatalf("clean save: err=%v exit=%d", run.err, run.exitCode)
	}
	if run.decoded["status"] != "saved" || len(vaultStashDirs(t, run.vault)) != 1 {
		t.Fatalf("output = %v", run.decoded)
	}
}

func TestSaveFailOnSecretsConflictsWithNoScan(t *testing.T) {
	source := t.TempDir()
	writeFiles(t, source, map[string]string{"clean.txt": "harmless\n"})
	run := runSave(t, source, false, func() { saveFailOnSecrets, saveNoScan = true, true })
	if run.err == nil || !strings.Contains(run.err.Error(), "--no-scan") {
		t.Fatalf("err = %v, want a --no-scan conflict", run.err)
	}
	if run.exitCode != -1 || len(vaultStashDirs(t, run.vault)) != 0 {
		t.Fatalf("exit=%d dirs=%v", run.exitCode, vaultStashDirs(t, run.vault))
	}
}

func TestSaveScansTraceZipMembersAndNamesThemArchiveBangMember(t *testing.T) {
	source := t.TempDir()
	writeFiles(t, source, map[string]string{"clean.txt": "harmless\n"})
	zipPath := filepath.Join(source, "trace.zip")
	writeTestZip(t, zipPath, map[string]string{
		"trace.network": `{"headers":[{"name":"Authorization","value":"Bearer abcdefghijklmnopqrstuvwx"}]}` + "\n",
		"shot.png":      "not scanned",
	})
	run := runSave(t, source, true, nil)
	if run.err != nil {
		t.Fatal(run.err)
	}
	if strings.Contains(run.stdout, "abcdefghijklmnopqrstuvwx") {
		t.Fatal("JSON must never carry a token value")
	}
	findings := run.decoded["secrets"].(map[string]any)["findings"].([]any)
	if len(findings) != 1 {
		t.Fatalf("findings = %v", findings)
	}
	if f := findings[0].(map[string]any); f["file"] != "trace.zip!trace.network" || f["rule"] != "bearer-token" || f["line"].(float64) != 1 {
		t.Fatalf("finding = %v", f)
	}
}

func writeTestZip(t *testing.T, path string, members map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range members {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
