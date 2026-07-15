package docs

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestEmbeddedPagesListAndRead(t *testing.T) {
	pages := List()
	if !slices.IsSorted(pages) {
		t.Fatalf("List() is not sorted: %v", pages)
	}
	for _, expected := range []string{
		"cli/completion.md",
		"cli/ecosystem-status.md",
		"cli/index.md",
		"cli/save.md",
		"guide/agent-guide.md",
		"guide/core-concepts.md",
		"guide/getting-started.md",
		"guide/index.md",
		"guide/troubleshooting.md",
		"mcp/overview.md",
	} {
		if !slices.Contains(pages, expected) {
			t.Fatalf("List() is missing %q: %v", expected, pages)
		}
	}

	for _, name := range []string{"cli/save", "cli/save.md", "guide/agent-guide"} {
		page, err := Read(name)
		if err != nil {
			t.Fatalf("Read(%q): %v", name, err)
		}
		if page.Content == "" {
			t.Fatalf("Read(%q) = %+v, want non-empty page", name, page)
		}
	}

	agentGuide, err := Read("guide/agent-guide.md")
	if err != nil {
		t.Fatal(err)
	}
	if agentGuide.Name != "guide/agent-guide" || !strings.Contains(agentGuide.Content, "fcheap agent --json") {
		t.Fatalf("agent guide = %+v, want canonical version-matched guide", agentGuide)
	}
}

func TestEmbeddedPagesRejectTraversal(t *testing.T) {
	for _, name := range []string{"", "../README", "cli/../index", "/index", `..\README`} {
		if _, err := Read(name); !errors.Is(err, ErrInvalidPage) {
			t.Fatalf("Read(%q) error = %v, want ErrInvalidPage", name, err)
		}
	}
}

func TestEmbeddedPageNotFound(t *testing.T) {
	if _, err := Read("cli/does-not-exist"); !errors.Is(err, ErrPageNotFound) {
		t.Fatalf("Read missing page error = %v, want ErrPageNotFound", err)
	}
}
