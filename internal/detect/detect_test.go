package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const vidtraceTimeline = `{
  "schema_version": "1",
  "entries": [
    {"time_seconds": 0, "frame": "frames/f1.png",
     "ocr": {"text": "NullPointerException at checkout"},
     "transcript": [{"text": "the app crashed when I clicked pay"}]},
    {"time_seconds": 12, "frame": "frames/f2.png",
     "ocr": {"text": "Retry button shown"},
     "transcript": []}
  ]
}`

func writeBundle(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"),
		[]byte(`{"schema_version":"1","source_video":"/Downloads/bug.mp4"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "timeline.json"), []byte(vidtraceTimeline), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDetectVidtrace(t *testing.T) {
	dir := writeBundle(t)
	r := Detect(dir)

	if r.Type != TypeVidtrace {
		t.Fatalf("Type = %q, want vidtrace", r.Type)
	}
	// The OCR and transcript text must be indexed (regression: the parser used
	// to expect the wrong schema and indexed nothing).
	for _, want := range []string{"NullPointerException at checkout", "clicked pay", "Retry button shown"} {
		if !strings.Contains(r.SearchableText, want) {
			t.Errorf("SearchableText missing %q", want)
		}
	}
	// Per-entry units carry the frame + timestamp label.
	if len(r.Units) != 2 {
		t.Fatalf("Units = %d, want 2", len(r.Units))
	}
	if !strings.Contains(r.Units[0].Label, "frames/f1.png") || !strings.Contains(r.Units[0].Label, "0s") {
		t.Errorf("unit label = %q, want frame@time", r.Units[0].Label)
	}
}

func TestParseVidtraceTimeline(t *testing.T) {
	dir := writeBundle(t)
	entries := ParseVidtraceTimeline(dir)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[0].TimeSeconds != 0 || entries[0].Frame != "frames/f1.png" {
		t.Errorf("entry[0] = %+v", entries[0])
	}
	if entries[0].OCR != "NullPointerException at checkout" {
		t.Errorf("entry[0].OCR = %q", entries[0].OCR)
	}
	if !strings.Contains(entries[0].Transcript, "clicked pay") {
		t.Errorf("entry[0].Transcript = %q", entries[0].Transcript)
	}
	if entries[1].Transcript != "" {
		t.Errorf("entry[1].Transcript should be empty, got %q", entries[1].Transcript)
	}
}

func TestParseVidtraceTimelineMissing(t *testing.T) {
	if got := ParseVidtraceTimeline(t.TempDir()); got != nil {
		t.Errorf("expected nil for missing timeline, got %v", got)
	}
}

func TestBundleTypeOf(t *testing.T) {
	if got := BundleTypeOf(writeBundle(t)); got != TypeVidtrace {
		t.Errorf("BundleTypeOf(bundle) = %q, want vidtrace", got)
	}
	if got := BundleTypeOf(t.TempDir()); got != TypeGeneric {
		t.Errorf("BundleTypeOf(empty) = %q, want generic", got)
	}
}

func TestVidtraceMetadata(t *testing.T) {
	meta, ok := VidtraceMetadata(writeBundle(t))
	if !ok {
		t.Fatal("expected metadata, got ok=false")
	}
	if meta["source_video"] != "/Downloads/bug.mp4" {
		t.Errorf("source_video = %v", meta["source_video"])
	}
	if _, ok := VidtraceMetadata(t.TempDir()); ok {
		t.Error("expected ok=false for missing metadata.json")
	}
}

// TestDetectVidtraceFromFixture exercises detection against the committed golden
// bundle in testdata/ (rather than an inline fixture), so the on-disk sample
// stays valid as the schema evolves.
func TestDetectVidtraceFromFixture(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "sample_bundle")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("fixture not present: %v", err)
	}

	r := Detect(dir)
	if r.Type != TypeVidtrace {
		t.Fatalf("Type = %q, want vidtrace", r.Type)
	}
	for _, want := range []string{
		"login button not responding", // entry 1 OCR
		"I click login",               // entry 1 transcript
		"error 500 internal server",   // entry 2 OCR
		"test.mp4",                    // source_video from metadata.json
	} {
		if !strings.Contains(r.SearchableText, want) {
			t.Errorf("SearchableText missing %q", want)
		}
	}
	if len(r.Units) != 2 {
		t.Errorf("Units = %d, want 2", len(r.Units))
	}
	// When the timeline yields per-frame units, the raw ocr/ and transcript/
	// files must NOT also be indexed — that text is already covered per frame, so
	// re-indexing the files would double-count it in BM25.
	if len(r.SearchableFiles) != 0 {
		t.Errorf("SearchableFiles = %d, want 0 (timeline units already cover ocr/transcript)", len(r.SearchableFiles))
	}

	meta, ok := VidtraceMetadata(dir)
	if !ok || meta["source_video"] != "test.mp4" {
		t.Errorf("VidtraceMetadata = %v (ok=%v), want source_video=test.mp4", meta, ok)
	}
}

func TestDetectGeneric(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("a bug report about login"), 0644); err != nil {
		t.Fatal(err)
	}
	r := Detect(dir)
	if r.Type != TypeGeneric {
		t.Errorf("Type = %q, want generic", r.Type)
	}
	if len(r.SearchableFiles) == 0 {
		t.Error("expected at least one searchable file")
	}
}
