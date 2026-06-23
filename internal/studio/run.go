// Package studio implements the fcheap studio TUI for browsing, searching,
// and acting on stashes -- a themed, responsive Bubble Tea (v2) interface.
package studio

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
)

// Run starts the studio TUI. It requires an interactive terminal.
//
// stashDir is the root directory holding stashes; vecgrepPath is the optional
// path to a vecgrep binary used for semantic search; emb configures the optional
// embedder for semantic/hybrid search (may be empty for BM25-only).
func Run(stashDir, vecgrepPath string, emb analyze.EmbedderSettings) error {
	if !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("studio requires an interactive terminal")
	}

	model := NewModel(context.Background(), stashDir, vecgrepPath, emb)
	_, err := tea.NewProgram(model).Run()
	return err
}
