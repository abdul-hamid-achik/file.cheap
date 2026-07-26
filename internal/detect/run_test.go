package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectCairntraceRun(t *testing.T) {
	dir := filepath.Join("testdata", "cairntrace-run")
	result := Detect(dir)

	if result.Type != TypeCairntraceRun {
		t.Fatalf("Type = %q, want %q", result.Type, TypeCairntraceRun)
	}
	assertRunMetadata(t, result, map[string]any{
		"artifact_count": 2,
		"backend":        "agent-browser",
		"environment":    "test",
		"exit_code":      int64(1),
		"outcome_count":  1,
		"run_id":         "2026-07-01T12-00-00Z_checkout_alpha",
		"spec_name":      "checkout_alpha",
		"status":         "failed",
		"step_count":     1,
	})
	assertNoSensitiveRunPayload(t, result)
	if got := BundleTypeOf(dir); got != TypeCairntraceRun {
		t.Fatalf("BundleTypeOf = %q, want %q", got, TypeCairntraceRun)
	}
}

func TestDetectGlyphrunRunSchemaDrift(t *testing.T) {
	tests := []struct {
		name          string
		dir           string
		status        string
		errorKind     string
		artifactCount int
	}{
		{name: "legacy numeric schema version", dir: "glyphrun-run-legacy", status: "passed", artifactCount: 2},
		{name: "current schema URI", dir: "glyphrun-run-current", status: "errored", errorKind: "timeout", artifactCount: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join("testdata", tt.dir)
			result := Detect(dir)
			if result.Type != TypeGlyphrunRun {
				t.Fatalf("Type = %q, want %q", result.Type, TypeGlyphrunRun)
			}
			assertRunMetadata(t, result, map[string]any{
				"artifact_count": tt.artifactCount,
				"error_kind":     tt.errorKind,
				"status":         tt.status,
			})
			assertNoSensitiveRunPayload(t, result)
			if got := BundleTypeOf(dir); got != TypeGlyphrunRun {
				t.Fatalf("BundleTypeOf = %q, want %q", got, TypeGlyphrunRun)
			}
		})
	}
}

func TestDetectCairntraceManifestWithoutRun(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "artifact-manifest.json"), []byte(`{"version":1,"artifacts":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	result := Detect(dir)
	if result.Type != TypeCairntraceRun {
		t.Fatalf("Type = %q, want incomplete %q", result.Type, TypeCairntraceRun)
	}
	if result.Metadata["artifact_count"] != 0 {
		t.Errorf("artifact_count = %v, want 0", result.Metadata["artifact_count"])
	}
}

func TestNativeRunMalformedMetadataFailsClosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(`{"diagnostic":"DO_NOT_INDEX_DIAGNOSTIC_PAYLOAD"`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`[]`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "diagnostic.md"), []byte("DO_NOT_INDEX_DIAGNOSTIC_PAYLOAD"), 0600); err != nil {
		t.Fatal(err)
	}
	result := Detect(dir)
	if result.Type != TypeGlyphrunRun {
		t.Fatalf("Type = %q, want fail-closed %q", result.Type, TypeGlyphrunRun)
	}
	assertNoSensitiveRunPayload(t, result)
}

func TestRunJSONWithoutNativeMarkersRemainsGeneric(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run.json"), []byte(`{"runId":"generic","status":"passed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := Detect(dir).Type; got != TypeGeneric {
		t.Fatalf("Type = %q, want %q", got, TypeGeneric)
	}
}

func assertRunMetadata(t *testing.T, result Result, expected map[string]any) {
	t.Helper()
	for key, want := range expected {
		if want == "" {
			if _, exists := result.Metadata[key]; exists {
				t.Errorf("Metadata[%q] = %v, want absent", key, result.Metadata[key])
			}
			continue
		}
		if got := result.Metadata[key]; got != want {
			t.Errorf("Metadata[%q] = %#v, want %#v", key, got, want)
		}
	}
}

func assertNoSensitiveRunPayload(t *testing.T, result Result) {
	t.Helper()
	encoded, err := json.Marshal(result.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	visible := result.SearchableText + string(encoded)
	for _, forbidden := range []string{
		"DO_NOT_INDEX_DIAGNOSTIC_PAYLOAD",
		"DO_NOT_INDEX_FAILURE_DETAIL",
		"DO_NOT_INDEX_INTENT_PAYLOAD",
		"DO_NOT_INDEX_NEXT_ACTION",
		"DO_NOT_INDEX_OUTCOME_EVIDENCE",
		"DO_NOT_INDEX_STEP_ACTION",
		"DO_NOT_INDEX_STEP_DETAIL",
		"DO_NOT_INDEX_SUMMARY_PAYLOAD",
		"DO_NOT_INDEX_TARGET_COMMAND",
		"raw/private.log",
		"raw/pty.raw.log",
	} {
		if strings.Contains(visible, forbidden) {
			t.Errorf("detector exposed sensitive payload marker %q", forbidden)
		}
	}
	if len(result.SearchableFiles) != 0 {
		t.Errorf("SearchableFiles = %v, want none for native run", result.SearchableFiles)
	}
}
