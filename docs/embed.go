// Package docs exposes the public Markdown documentation embedded in the fcheap
// binary. Read-only CLI and MCP documentation access therefore works from an
// installed release without depending on the caller's working directory.
package docs

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

var (
	ErrInvalidPage  = errors.New("invalid documentation page")
	ErrPageNotFound = errors.New("documentation page not found")
)

// Page is one embedded documentation page. Name is canonical and omits the
// .md extension so callers can safely echo it in structured output.
type Page struct {
	Name    string
	Content string
}

// Keep every first-level Markdown section available to installed CLI and MCP
// clients. A new public section does not need a parallel hard-coded pattern.
//
//go:embed *.md */*.md
var content embed.FS

var pageNames = listPages()

// List returns all embedded Markdown paths, including their .md extension.
func List() []string {
	return append([]string(nil), pageNames...)
}

// Read returns one embedded page by canonical relative name, with or without a
// .md suffix. Absolute, traversal, backslash, and non-canonical paths are
// rejected before the embedded filesystem is consulted.
func Read(name string) (Page, error) {
	canonical, err := canonicalName(name)
	if err != nil {
		return Page{}, err
	}
	data, err := content.ReadFile(canonical + ".md")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Page{}, fmt.Errorf("%w: %s", ErrPageNotFound, canonical)
		}
		return Page{}, fmt.Errorf("read embedded documentation page %q: %w", canonical, err)
	}
	return Page{Name: canonical, Content: string(data)}, nil
}

func canonicalName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".md")
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) || strings.ContainsRune(name, '\x00') {
		return "", fmt.Errorf("%w: %q", ErrInvalidPage, name)
	}
	clean := path.Clean(name)
	if clean != name || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q", ErrInvalidPage, name)
	}
	return clean, nil
}

func listPages() []string {
	pages := []string{}
	_ = fs.WalkDir(content, ".", func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(filePath, ".md") {
			return nil
		}
		pages = append(pages, filePath)
		return nil
	})
	sort.Strings(pages)
	return pages
}
