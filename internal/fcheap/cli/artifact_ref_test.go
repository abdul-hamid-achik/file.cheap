package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/artifactref"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/config"
	"github.com/abdul-hamid-achik/file.cheap/internal/fcheap/output"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

func TestArtifactRefCommandJSONContract(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "vault")
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(filepath.Join(source, "reports"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "reports", "report.json"), []byte(`{"status":"passed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	saved, err := mgr.Save(context.Background(), &stash.SaveOptions{SourcePath: source})
	if err != nil {
		t.Fatal(err)
	}

	restoreArtifactRefGlobals(t)
	var stdout bytes.Buffer
	cfg = &config.Config{StashDir: vault}
	printer = output.New(output.WithJSON(true), output.WithOutput(&stdout), output.WithNoColor(true))
	artifactRefKind = "chalupa.report"
	artifactRefProducerTool = "chalupa"
	artifactRefProducerVersion = "0.9.0"
	artifactRefNativeSchema = "urn:chalupa.dev:report:v1"
	artifactRefNativeID = "report_01"
	artifactRefEntrypoint = "reports/report.json"

	if err := artifactRefCmd.RunE(artifactRefCmd, []string{saved.Manifest.ID}); err != nil {
		t.Fatalf("artifact-ref: %v", err)
	}
	var got artifactref.ArtifactRefV1
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON %q: %v", stdout.String(), err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.ArtifactID != saved.Manifest.ID || got.URI != artifactref.LocalURI(saved.Manifest.ID) {
		t.Fatalf("local identity = %+v", got)
	}
	if got.Kind != "chalupa.report" || got.Producer == nil ||
		got.Producer.Tool != "chalupa" || got.Producer.Entrypoint != "reports/report.json" {
		t.Fatalf("metadata = %+v", got)
	}
	for _, forbidden := range []string{`"integrity"`, `"web_url"`} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("JSON contains %s: %s", forbidden, stdout.String())
		}
	}
}

func TestArtifactRefCommandIsRegisteredWithStableFlags(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"artifact-ref"})
	if err != nil || found != artifactRefCmd {
		t.Fatalf("root artifact-ref command = (%v, %v), want artifactRefCmd", found, err)
	}
	for _, flag := range []string{
		"kind",
		"producer-tool",
		"producer-version",
		"native-schema",
		"native-id",
		"entrypoint",
	} {
		if artifactRefCmd.Flags().Lookup(flag) == nil {
			t.Errorf("artifact-ref missing --%s", flag)
		}
	}
	for _, want := range []string{
		"metadata pointer, not an upload",
		"fcheap artifact-ref <stash-id> --json",
		"--producer-tool cairntrace",
	} {
		if !strings.Contains(artifactRefCmd.Long+"\n"+artifactRefCmd.Example, want) {
			t.Errorf("artifact-ref help missing %q", want)
		}
	}
}

func TestArtifactRefCommandHumanDefaultsAndLoadsExistingStash(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "vault")
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(source, []byte("evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	saved, err := mgr.Save(context.Background(), &stash.SaveOptions{SourcePath: source})
	if err != nil {
		t.Fatal(err)
	}

	restoreArtifactRefGlobals(t)
	var stdout bytes.Buffer
	cfg = &config.Config{StashDir: vault}
	printer = output.New(output.WithOutput(&stdout), output.WithNoColor(true))

	if err := artifactRefCmd.RunE(artifactRefCmd, []string{saved.Manifest.ID}); err != nil {
		t.Fatalf("artifact-ref: %v", err)
	}
	for _, want := range []string{
		"Artifact Reference",
		"fcheap-local",
		artifactref.LocalURI(saved.Manifest.ID),
		"filecheap.stash",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("human output missing %q:\n%s", want, stdout.String())
		}
	}

	if err := artifactRefCmd.RunE(artifactRefCmd, []string{"missing-stash"}); err == nil {
		t.Fatal("artifact-ref accepted a stash that does not exist")
	}
}

func TestArtifactRefCommandRejectsPartialOrUnsafeProducer(t *testing.T) {
	vault := filepath.Join(t.TempDir(), "vault")
	mgr, err := stash.NewManager(vault)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "run.json")
	if err := os.WriteFile(source, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	saved, err := mgr.Save(context.Background(), &stash.SaveOptions{SourcePath: source})
	if err != nil {
		t.Fatal(err)
	}

	restoreArtifactRefGlobals(t)
	cfg = &config.Config{StashDir: vault}
	printer = output.New(output.WithJSON(true), output.WithOutput(&bytes.Buffer{}), output.WithNoColor(true))
	artifactRefEntrypoint = "../run.json"

	err = artifactRefCmd.RunE(artifactRefCmd, []string{saved.Manifest.ID})
	if err == nil || !strings.Contains(err.Error(), ".producer.tool") {
		t.Fatalf("error = %v, want producer tool validation", err)
	}

	artifactRefProducerTool = "cairntrace"
	err = artifactRefCmd.RunE(artifactRefCmd, []string{saved.Manifest.ID})
	if err == nil || !strings.Contains(err.Error(), ".producer.entrypoint") {
		t.Fatalf("error = %v, want entrypoint validation", err)
	}

	artifactRefEntrypoint = "missing.json"
	err = artifactRefCmd.RunE(artifactRefCmd, []string{saved.Manifest.ID})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("error = %v, want missing entrypoint validation", err)
	}
}

func restoreArtifactRefGlobals(t *testing.T) {
	t.Helper()
	oldCfg, oldPrinter := cfg, printer
	oldKind := artifactRefKind
	oldTool, oldVersion := artifactRefProducerTool, artifactRefProducerVersion
	oldSchema, oldID, oldEntrypoint := artifactRefNativeSchema, artifactRefNativeID, artifactRefEntrypoint
	t.Cleanup(func() {
		cfg, printer = oldCfg, oldPrinter
		artifactRefKind = oldKind
		artifactRefProducerTool, artifactRefProducerVersion = oldTool, oldVersion
		artifactRefNativeSchema, artifactRefNativeID, artifactRefEntrypoint = oldSchema, oldID, oldEntrypoint
	})
	artifactRefKind = ""
	artifactRefProducerTool = ""
	artifactRefProducerVersion = ""
	artifactRefNativeSchema = ""
	artifactRefNativeID = ""
	artifactRefEntrypoint = ""
}
