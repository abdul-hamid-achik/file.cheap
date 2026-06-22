// Package studio implements the fcheap Studio TUI for browsing stashes.
package studio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
	"github.com/charmbracelet/x/term"
)

// Run starts the Studio TUI.
func Run(stashDir, vecgrepPath string) error {
	if !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("studio requires an interactive terminal")
	}

	m := initialModel(stashDir, vecgrepPath)
	_, err := tea.NewProgram(m).Run()
	return err
}

type model struct {
	stashDir    string
	vecgrepPath string
	stashes     []*stash.Stash
	cursor      int
	width       int
	height      int
	err         error
	view        string // "list", "detail", "search", "help", "status"
	searchQuery string
	searchResults []analyze.SearchResult
	selected    *stash.Stash
	status      string
}

func initialModel(stashDir, vecgrepPath string) model {
	return model{
		stashDir:    stashDir,
		vecgrepPath: vecgrepPath,
		view:        "list",
	}
}

func (m model) Init() tea.Cmd {
	return loadStashes(m.stashDir)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case stashesLoadedMsg:
		m.stashes = msg.stashes
		m.status = fmt.Sprintf("%d stash(es)", len(m.stashes))
		return m, nil

	case searchDoneMsg:
		m.searchResults = msg.results
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc":
			if m.view != "list" {
				m.view = "list"
				return m, nil
			}
			return m, tea.Quit
		case "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if m.view == "list" && m.cursor < len(m.stashes)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.view == "list" && m.cursor > 0 {
				m.cursor--
			}
		case "enter", "l":
			if m.view == "list" && len(m.stashes) > 0 {
				m.selected = m.stashes[m.cursor]
				m.view = "detail"
			}
		case "h":
			if m.view == "detail" {
				m.view = "list"
			}
		case "s":
			if m.view == "list" {
				m.view = "search"
			}
		case "?":
			if m.view == "help" {
				m.view = "list"
			} else {
				m.view = "help"
			}
		case "r":
			if m.view == "list" {
				return m, loadStashes(m.stashDir)
			}
		}
	}

	return m, nil
}

func (m model) View() tea.View {
	var content string
	if m.err != nil {
		content = fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err)
	} else {
		switch m.view {
		case "list":
			content = m.listView()
		case "detail":
			content = m.detailView()
		case "search":
			content = m.searchView()
		case "help":
			content = m.helpView()
		default:
			content = m.listView()
		}
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m model) listView() string {
	var sb strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render("fcheap Studio")
	sb.WriteString(title)
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(m.status))
	sb.WriteString("\n\n")

	if len(m.stashes) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("No stashes found. Use 'fcheap save <path>' to create one.\n"))
		sb.WriteString(m.helpFooter())
		return sb.String()
	}

	for i, st := range m.stashes {
		cursor := "  "
		if i == m.cursor {
			cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render("▸ ")
		}
		name := st.Manifest.Name
		if name == "" {
			name = st.Manifest.ID
		}
		line := fmt.Sprintf("%s%s (%d files, %s)", cursor, name, st.Manifest.FileCount, formatSize(st.Manifest.TotalSize))
		if st.Manifest.Tool != "" {
			line += fmt.Sprintf(" [%s]", st.Manifest.Tool)
		}
		if i == m.cursor {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(m.helpFooter())
	return sb.String()
}

func (m model) detailView() string {
	if m.selected == nil {
		return "No stash selected\n\nPress q to go back."
	}

	man := m.selected.Manifest
	var sb strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render("Stash Detail")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("  ID:       %s\n", man.ID))
	if man.Name != "" {
		sb.WriteString(fmt.Sprintf("  Name:     %s\n", man.Name))
	}
	sb.WriteString(fmt.Sprintf("  Created:  %s\n", man.CreatedAt))
	if man.SourcePath != "" {
		sb.WriteString(fmt.Sprintf("  Source:   %s\n", man.SourcePath))
	}
	if man.Tool != "" {
		sb.WriteString(fmt.Sprintf("  Tool:     %s\n", man.Tool))
	}
	if man.BundleType != "" && man.BundleType != "generic" {
		sb.WriteString(fmt.Sprintf("  Bundle:   %s\n", man.BundleType))
	}
	sb.WriteString(fmt.Sprintf("  Files:    %d\n", man.FileCount))
	sb.WriteString(fmt.Sprintf("  Size:     %s\n", formatSize(man.TotalSize)))
	sb.WriteString(fmt.Sprintf("  Hash:     %s\n", man.ContentHash))
	if len(man.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("  Tags:     %v\n", man.Tags))
	}

	if len(man.Files) > 0 {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Bold(true).Render("Files:"))
		sb.WriteString("\n")
		maxShow := 30
		for i, f := range man.Files {
			if i >= maxShow {
				sb.WriteString(fmt.Sprintf("  ... and %d more\n", len(man.Files)-maxShow))
				break
			}
			sb.WriteString(fmt.Sprintf("  %s (%s)\n", f.Path, formatSize(f.Size)))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("[h] back  [q] quit"))
	sb.WriteString("\n")
	return sb.String()
}

func (m model) searchView() string {
	var sb strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render("Search")
	sb.WriteString(title)
	sb.WriteString("\n\n")

	if m.searchQuery == "" {
		sb.WriteString("Type a search query (this is a basic implementation)\n")
	}

	if len(m.searchResults) > 0 {
		for _, r := range m.searchResults {
			sb.WriteString(fmt.Sprintf("  %s (score: %.2f)\n", r.StashID, r.Score))
			text := r.Text
			if len(text) > 100 {
				text = text[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("    %s\n", text))
		}
	} else {
		sb.WriteString("No results. Run 'fcheap analyze <stash-id>' to index stashes first.\n")
	}

	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("[q] back  [esc] quit"))
	sb.WriteString("\n")
	return sb.String()
}

func (m model) helpView() string {
	var sb strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Render("Help")
	sb.WriteString(title)
	sb.WriteString("\n\n")
	sb.WriteString("  j/k      Move cursor down/up\n")
	sb.WriteString("  enter    View stash detail\n")
	sb.WriteString("  h        Go back to list\n")
	sb.WriteString("  r        Refresh stash list\n")
	sb.WriteString("  s        Search mode\n")
	sb.WriteString("  ?        Toggle this help\n")
	sb.WriteString("  q/esc    Quit (or go back from sub-view)\n")
	sb.WriteString("  ctrl+c   Force quit\n")
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("Press ? or q to close help"))
	sb.WriteString("\n")
	return sb.String()
}

func (m model) helpFooter() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("[enter] view  [s] search  [r] refresh  [?] help  [q] quit")
}

// --- messages ---

type stashesLoadedMsg struct {
	stashes []*stash.Stash
}

type searchDoneMsg struct {
	results []analyze.SearchResult
}

// --- commands ---

func loadStashes(stashDir string) tea.Cmd {
	return func() tea.Msg {
		mgr, err := stash.NewManager(stashDir)
		if err != nil {
			return errMsg{err: err}
		}
		stashes, err := mgr.List(context.Background(), "")
		if err != nil {
			return errMsg{err: err}
		}
		return stashesLoadedMsg{stashes: stashes}
	}
}

type errMsg struct {
	err error
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// unused import guard
var _ = filepath.Join