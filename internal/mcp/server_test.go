package mcp

import (
	"slices"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/cleanup"
)

func TestTextResultIncludesObjectStructuredContent(t *testing.T) {
	result := textResult(map[string]any{"status": "ok"})
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["status"] != "ok" {
		t.Fatalf("StructuredContent = %#v, want object result", result.StructuredContent)
	}
	if len(result.Content) != 1 {
		t.Fatalf("Content length = %d, want backwards-compatible JSON text", len(result.Content))
	}
}

func TestTextResultWrapsArrayStructuredContent(t *testing.T) {
	result := textResult([]string{"one", "two"})
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("StructuredContent = %#v, want object wrapper", result.StructuredContent)
	}
	items, ok := structured["result"].([]string)
	if !ok || len(items) != 2 {
		t.Fatalf("wrapped result = %#v, want original array", structured["result"])
	}
}

func TestDocsUseEmbeddedContentAndRejectTraversal(t *testing.T) {
	t.Chdir(t.TempDir())
	pages := listDocPages()
	if !slices.Contains(pages, "cli/save.md") {
		t.Fatalf("listDocPages() missing cli/save.md: %v", pages)
	}
	content, err := readDocPage("cli/save")
	if err != nil {
		t.Fatalf("readDocPage embedded content: %v", err)
	}
	if !strings.Contains(content, "# save") {
		t.Fatalf("readDocPage content = %q, want save docs", content)
	}
	if _, err := readDocPage("../README"); err == nil {
		t.Fatal("readDocPage traversal succeeded, want error")
	}
}

func TestScoringCleanupResultMarksPartialFailureAsToolError(t *testing.T) {
	result := &cleanup.Result{
		Candidates: []cleanup.Candidate{},
		Dropped:    []string{"stash-1"},
		Skipped:    []string{},
		Failed: []cleanup.Failure{{
			ID: "stash-1", Stage: "index", Error: "index unavailable",
		}},
		Applied: true,
	}

	toolResult := scoringCleanupResult(result)
	if !toolResult.IsError {
		t.Fatal("partial scoring-cleanup failure did not set IsError")
	}
	structured, ok := toolResult.StructuredContent.(*cleanup.Result)
	if !ok || len(structured.Failed) != 1 || structured.Failed[0].Stage != "index" {
		t.Fatalf("StructuredContent = %#v, want cleanup failure details", toolResult.StructuredContent)
	}
}
