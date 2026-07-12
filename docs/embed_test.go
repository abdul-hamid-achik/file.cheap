package docs

import (
	"errors"
	"slices"
	"testing"
)

func TestEmbeddedPagesListAndRead(t *testing.T) {
	pages := List()
	if !slices.IsSorted(pages) {
		t.Fatalf("List() is not sorted: %v", pages)
	}
	if !slices.Contains(pages, "cli/save.md") || !slices.Contains(pages, "mcp/overview.md") {
		t.Fatalf("List() is missing expected pages: %v", pages)
	}

	for _, name := range []string{"cli/save", "cli/save.md"} {
		page, err := Read(name)
		if err != nil {
			t.Fatalf("Read(%q): %v", name, err)
		}
		if page.Name != "cli/save" || page.Content == "" {
			t.Fatalf("Read(%q) = %+v, want canonical non-empty page", name, page)
		}
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
