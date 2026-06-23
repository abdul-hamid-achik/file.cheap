package studio

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

// viewName enumerates the top-level screens.
type viewName int

const (
	viewList viewName = iota
	viewDetail
	viewSearch
	viewStatus
	viewHelp
	viewTimeline
	viewDiff
)

// focusArea tracks which pane receives navigation/input in multi-pane views.
type focusArea int

const (
	focusList focusArea = iota
	focusFiles
	focusPreview
	focusQuery
	focusResults
)

// confirmAction is a pending destructive action awaiting y/n.
type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmDrop
)

// Model is the studio TUI state.
type Model struct {
	ctx         context.Context
	stashDir    string
	vecgrepPath string

	analyzer *analyze.Analyzer

	width  int
	height int

	activeView viewName
	focus      focusArea

	// list view
	stashes []*stash.Stash
	cursor  int
	loading bool

	// detail view
	selected  *stash.Stash
	fileIdx   int
	preview   viewport.Model
	previewID string // stash id whose preview is loaded

	// search view
	query         textinput.Model
	searchResults []analyze.SearchResult
	resultIdx     int
	searching     bool
	lastQuery     string
	searchMode    string // "auto" | "keyword" | "semantic" | "hybrid"

	// transient feedback
	statusMessage string
	errMessage    string
	confirm       confirmAction
	indexing      bool
	working       bool // an action (restore/drop/compress) is in flight
	spinner       spinner.Model
	progress      progress.Model
	indexDone     int
	indexTotal    int

	// diff view
	diffInput     textinput.Model
	diffPrompting bool
}

// busy reports whether any async operation is in flight (drives the spinner).
func (m Model) busy() bool {
	return m.loading || m.searching || m.indexing || m.working
}

// --- message types (every async cmd resolves to one of these) ---

type stashesLoadedMsg struct {
	stashes []*stash.Stash
	err     error
}

type searchDoneMsg struct {
	query   string
	results []analyze.SearchResult
	err     error
}

type previewLoadedMsg struct {
	stashID string
	title   string
	content string
	err     error
}

type actionDoneMsg struct {
	kind    string // "restore", "drop", "compress"
	message string
	err     error
}

type indexDoneMsg struct {
	message string
	err     error
}

type timelineLoadedMsg struct {
	count   int
	content string
	err     error
}

type indexProg struct {
	done, total int
}

type indexProgressMsg struct {
	done, total int
	ch          chan indexProg
}

type indexProgressClosedMsg struct{}

type diffDoneMsg struct {
	content string
	err     error
}

// NewModel constructs the initial studio model.
func NewModel(ctx context.Context, stashDir, vecgrepPath string, emb analyze.EmbedderSettings) Model {
	ti := textinput.New()
	ti.Placeholder = "search stash content by keyword..."
	ti.Prompt = "› "
	ti.SetWidth(60)

	styles := textinput.DefaultDarkStyles()
	styles.Focused.Prompt = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	styles.Focused.Text = lipgloss.NewStyle().Foreground(colorInk)
	styles.Blurred.Prompt = lipgloss.NewStyle().Foreground(colorMuted)
	styles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	styles.Blurred.Text = lipgloss.NewStyle().Foreground(colorDim)
	ti.SetStyles(styles)

	di := textinput.New()
	di.Placeholder = "directory to diff against..."
	di.Prompt = "› "
	di.SetWidth(60)
	di.SetStyles(styles)

	vp := viewport.New(viewport.WithWidth(60), viewport.WithHeight(16))

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	prog := progress.New(progress.WithDefaultBlend(), progress.WithWidth(34))

	return Model{
		progress:    prog,
		ctx:         ctx,
		stashDir:    stashDir,
		vecgrepPath: vecgrepPath,
		analyzer:    analyze.NewAnalyzer(stashDir, vecgrepPath).WithEmbedder(emb),
		activeView:  viewList,
		focus:       focusList,
		query:       ti,
		diffInput:   di,
		preview:     vp,
		spinner:     sp,
		searchMode:  "auto",
		loading:     true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadStashesCmd(m.stashDir), m.spinner.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return m, cmd

	case indexProgressMsg:
		m.indexDone, m.indexTotal = msg.done, msg.total
		var pct float64
		if msg.total > 0 {
			pct = float64(msg.done) / float64(msg.total)
		}
		return m, tea.Batch(m.progress.SetPercent(pct), waitForIndexProgress(msg.ch))

	case indexProgressClosedMsg:
		return m, nil

	case diffDoneMsg:
		m.working = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.errMessage = ""
		m.preview.SetContent(msg.content)
		m.preview.GotoTop()
		m.activeView = viewDiff
		m.focus = focusPreview
		m.statusMessage = "diff complete"
		return m, nil

	case stashesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.stashes = msg.stashes
		m.errMessage = ""
		m.statusMessage = fmt.Sprintf("%d stash(es)", len(m.stashes))
		if m.cursor >= len(m.stashes) {
			m.cursor = clamp(len(m.stashes)-1, 0, len(m.stashes))
		}
		// Re-sync the selected stash after a reload (drop/compress): refresh it
		// from the new list, or fall back to the list view if it's gone.
		if m.selected != nil && m.selected.Manifest != nil {
			id := m.selected.Manifest.ID
			m.selected = nil
			for _, st := range m.stashes {
				if st.Manifest != nil && st.Manifest.ID == id {
					m.selected = st
					break
				}
			}
			if m.selected == nil && m.activeView == viewDetail {
				m.toList()
			}
		}
		return m, nil

	case searchDoneMsg:
		m.searching = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.searchResults = msg.results
		m.resultIdx = 0
		m.lastQuery = msg.query
		m.errMessage = ""
		m.statusMessage = fmt.Sprintf("%d match(es) for %q", len(msg.results), msg.query)
		return m, m.loadResultPreviewCmd()

	case previewLoadedMsg:
		if msg.err != nil {
			m.preview.SetContent(warnStyle.Render(msg.err.Error()))
			m.preview.GotoTop()
			return m, nil
		}
		m.previewID = msg.stashID
		m.preview.SetContent(msg.content)
		m.preview.GotoTop()
		return m, nil

	case actionDoneMsg:
		m.working = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.errMessage = ""
		m.statusMessage = msg.message
		// destructive/structural actions change the stash set on disk
		if msg.kind == "drop" || msg.kind == "compress" {
			return m, loadStashesCmd(m.stashDir)
		}
		return m, nil

	case indexDoneMsg:
		m.indexing = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.errMessage = ""
		m.statusMessage = msg.message
		return m, nil

	case timelineLoadedMsg:
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.errMessage = ""
		m.preview.SetContent(msg.content)
		m.preview.GotoTop()
		m.activeView = viewTimeline
		m.focus = focusPreview
		m.statusMessage = fmt.Sprintf("%d timeline entr%s", msg.count, plural(msg.count, "y", "ies"))
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil

	case tea.KeyPressMsg:
		cmd, handled := m.handleKey(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if handled {
			return m, tea.Batch(cmds...)
		}
	}

	// Route unhandled messages to the focused interactive component.
	if m.confirm == confirmNone {
		var cmd tea.Cmd
		switch {
		case m.diffPrompting:
			m.diffInput, cmd = m.diffInput.Update(msg)
		case m.activeView == viewSearch && m.focus == focusQuery:
			m.query, cmd = m.query.Update(msg)
		case m.focus == focusPreview:
			m.preview, cmd = m.preview.Update(msg)
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// handleKey processes a keypress. It returns a command and whether the key was
// fully handled (so the caller skips component routing).
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	key := msg.String()

	// Global force-quit.
	if key == "ctrl+c" {
		return tea.Quit, true
	}

	// Confirm flow takes priority everywhere.
	if m.confirm != confirmNone {
		switch key {
		case "y", "Y":
			action := m.confirm
			m.confirm = confirmNone
			if action == confirmDrop {
				return m.dropCmd(), true
			}
		case "n", "N", "esc":
			m.confirm = confirmNone
			m.statusMessage = "cancelled"
			return nil, true
		}
		return nil, true
	}

	// Diff path prompt intercepts keys until enter/esc.
	if m.diffPrompting {
		switch key {
		case "enter":
			path := strings.TrimSpace(m.diffInput.Value())
			m.diffPrompting = false
			m.diffInput.Blur()
			if path == "" {
				m.statusMessage = "diff cancelled"
				return nil, true
			}
			return m.diffCmd(path), true
		case "esc":
			m.diffPrompting = false
			m.diffInput.Blur()
			m.statusMessage = "diff cancelled"
			return nil, true
		}
		return nil, false // let the textinput consume the keystroke
	}

	// When typing in the query field, intercept only control keys.
	if m.activeView == viewSearch && m.focus == focusQuery {
		switch key {
		case "enter":
			return m.searchCmd(), true
		case "esc":
			if len(m.searchResults) > 0 {
				m.focus = focusResults
			} else {
				m.toList()
			}
			m.query.Blur()
			return nil, true
		case "tab":
			m.cycleSearchFocus()
			return nil, true
		}
		return nil, false // let the textinput consume the keystroke
	}

	switch m.activeView {
	case viewList:
		return m.handleListKey(key)
	case viewDetail:
		return m.handleDetailKey(key)
	case viewSearch:
		return m.handleSearchKey(key)
	case viewTimeline:
		return m.handleTimelineKey(key)
	case viewDiff:
		return m.handleDiffKey(key)
	case viewStatus, viewHelp:
		switch key {
		case "q", "esc", "?", "s", "enter":
			m.toList()
			return nil, true
		}
	}
	return nil, false
}

func (m *Model) handleListKey(key string) (tea.Cmd, bool) {
	switch key {
	case "q", "esc":
		return tea.Quit, true
	case "j", "down":
		if m.cursor < len(m.stashes)-1 {
			m.cursor++
		}
		return nil, true
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return nil, true
	case "enter", "l":
		if len(m.stashes) > 0 {
			m.openDetail()
			return m.loadFilePreviewCmd(), true
		}
		return nil, true
	case "/":
		m.openSearch()
		return nil, true
	case "s":
		m.activeView = viewStatus
		return nil, true
	case "?":
		m.activeView = viewHelp
		return nil, true
	case "g":
		return loadStashesCmd(m.stashDir), true
	case "r":
		return m.restoreCmd(), true
	case "c":
		return m.compressCmd(), true
	case "a":
		return m.indexCmd(), true
	case "x":
		if len(m.stashes) > 0 {
			return m.startDiffPrompt(), true
		}
		return nil, true
	case "d":
		if len(m.stashes) > 0 {
			m.confirm = confirmDrop
		}
		return nil, true
	}
	return nil, false
}

func (m *Model) handleDetailKey(key string) (tea.Cmd, bool) {
	files := m.selectedFiles()
	switch key {
	case "q":
		return tea.Quit, true
	case "esc", "h":
		m.toList()
		return nil, true
	case "tab":
		if m.focus == focusFiles {
			m.focus = focusPreview
		} else {
			m.focus = focusFiles
		}
		return nil, true
	case "j", "down":
		if m.focus == focusPreview {
			m.preview.ScrollDown(1)
			return nil, true
		}
		if m.fileIdx < len(files)-1 {
			m.fileIdx++
			return m.loadFilePreviewCmd(), true
		}
		return nil, true
	case "k", "up":
		if m.focus == focusPreview {
			m.preview.ScrollUp(1)
			return nil, true
		}
		if m.fileIdx > 0 {
			m.fileIdx--
			return m.loadFilePreviewCmd(), true
		}
		return nil, true
	case "pgdown", "ctrl+d":
		m.preview.HalfPageDown()
		return nil, true
	case "pgup", "ctrl+u":
		m.preview.HalfPageUp()
		return nil, true
	case "/":
		m.openSearch()
		return nil, true
	case "?":
		m.activeView = viewHelp
		return nil, true
	case "t":
		if m.selected != nil && m.selected.Manifest != nil && m.selected.Manifest.BundleType == "vidtrace" {
			return m.loadTimelineCmd(), true
		}
		m.statusMessage = "timeline view is only available for vidtrace bundles"
		return nil, true
	case "r":
		return m.restoreCmd(), true
	case "c":
		return m.compressCmd(), true
	case "a":
		return m.indexCmd(), true
	case "x":
		return m.startDiffPrompt(), true
	case "d":
		m.confirm = confirmDrop
		return nil, true
	}
	return nil, false
}

// startDiffPrompt focuses the diff path input, prefilled with the stash source.
func (m *Model) startDiffPrompt() tea.Cmd {
	m.diffPrompting = true
	m.statusMessage = ""
	if st := m.currentStash(); st != nil && st.Manifest != nil {
		m.diffInput.SetValue(st.Manifest.SourcePath)
	}
	return m.diffInput.Focus()
}

// handleDiffKey handles keys in the diff result view.
func (m *Model) handleDiffKey(key string) (tea.Cmd, bool) {
	switch key {
	case "q":
		return tea.Quit, true
	case "esc", "h":
		if m.selected != nil {
			m.activeView = viewDetail
			m.focus = focusFiles
		} else {
			m.toList()
		}
		return nil, true
	case "j", "down":
		m.preview.ScrollDown(1)
		return nil, true
	case "k", "up":
		m.preview.ScrollUp(1)
		return nil, true
	case "pgdown", "ctrl+d":
		m.preview.HalfPageDown()
		return nil, true
	case "pgup", "ctrl+u":
		m.preview.HalfPageUp()
		return nil, true
	case "?":
		m.activeView = viewHelp
		return nil, true
	}
	return nil, false
}

// handleTimelineKey handles keys in the vidtrace timeline view.
func (m *Model) handleTimelineKey(key string) (tea.Cmd, bool) {
	switch key {
	case "q":
		return tea.Quit, true
	case "esc", "h", "t":
		m.activeView = viewDetail
		m.focus = focusFiles
		return nil, true
	case "j", "down":
		m.preview.ScrollDown(1)
		return nil, true
	case "k", "up":
		m.preview.ScrollUp(1)
		return nil, true
	case "pgdown", "ctrl+d":
		m.preview.HalfPageDown()
		return nil, true
	case "pgup", "ctrl+u":
		m.preview.HalfPageUp()
		return nil, true
	case "?":
		m.activeView = viewHelp
		return nil, true
	}
	return nil, false
}

func (m *Model) handleSearchKey(key string) (tea.Cmd, bool) {
	switch key {
	case "q", "esc":
		m.toList()
		return nil, true
	case "/":
		m.focus = focusQuery
		return m.query.Focus(), true
	case "tab":
		m.cycleSearchFocus()
		return nil, true
	case "?":
		m.activeView = viewHelp
		return nil, true
	case "j", "down":
		if m.focus == focusPreview {
			m.preview.ScrollDown(1)
			return nil, true
		}
		if m.resultIdx < len(m.searchResults)-1 {
			m.resultIdx++
			return m.loadResultPreviewCmd(), true
		}
		return nil, true
	case "k", "up":
		if m.focus == focusPreview {
			m.preview.ScrollUp(1)
			return nil, true
		}
		if m.resultIdx > 0 {
			m.resultIdx--
			return m.loadResultPreviewCmd(), true
		}
		return nil, true
	case "m":
		m.cycleSearchMode()
		if strings.TrimSpace(m.query.Value()) != "" {
			return m.searchCmd(), true
		}
		m.statusMessage = "search mode: " + m.searchMode
		return nil, true
	case "enter":
		if m.focus == focusResults {
			return m.loadResultPreviewCmd(), true
		}
		m.focus = focusQuery
		return m.query.Focus(), true
	}
	return nil, false
}

// cycleSearchMode advances the search mode: auto -> keyword -> semantic -> hybrid.
func (m *Model) cycleSearchMode() {
	switch m.searchMode {
	case "auto":
		m.searchMode = "keyword"
	case "keyword":
		m.searchMode = "semantic"
	case "semantic":
		m.searchMode = "hybrid"
	default:
		m.searchMode = "auto"
	}
}

// --- view transitions ---

func (m *Model) toList() {
	m.activeView = viewList
	m.focus = focusList
	m.query.Blur()
	m.confirm = confirmNone
}

func (m *Model) openDetail() {
	m.selected = m.stashes[m.cursor]
	m.activeView = viewDetail
	m.focus = focusFiles
	m.fileIdx = 0
}

func (m *Model) openSearch() {
	m.activeView = viewSearch
	m.focus = focusQuery
	m.query.Focus()
}

func (m *Model) cycleSearchFocus() {
	switch m.focus {
	case focusQuery:
		m.focus = focusResults
		m.query.Blur()
	case focusResults:
		m.focus = focusPreview
	default:
		m.focus = focusQuery
		m.query.Focus()
	}
}

// resize recomputes component dimensions for the current terminal size.
func (m *Model) resize() {
	if m.width <= 0 {
		m.width = 100
	}
	if m.height <= 0 {
		m.height = 30
	}

	m.query.SetWidth(clamp(m.width-12, 20, 120))

	previewHeight := clamp(m.height-12, 6, 40)
	previewWidth := clamp(m.width/2-4, 30, m.width-4)
	if m.width < 96 {
		previewWidth = clamp(m.width-4, 30, m.width-4)
		previewHeight = clamp(m.height/2-6, 5, 30)
	}
	m.preview.SetWidth(previewWidth)
	m.preview.SetHeight(previewHeight)
}

// --- helpers ---

func (m Model) selectedFiles() []manifestFile {
	if m.selected == nil || m.selected.Manifest == nil {
		return nil
	}
	out := make([]manifestFile, 0, len(m.selected.Manifest.Files))
	for _, f := range m.selected.Manifest.Files {
		out = append(out, manifestFile{Path: f.Path, Size: f.Size})
	}
	return out
}

type manifestFile struct {
	Path string
	Size int64
}

// vecgrepAvailable reports whether semantic search backing is reachable.
func (m Model) vecgrepAvailable() bool {
	if strings.TrimSpace(m.vecgrepPath) != "" {
		return true
	}
	if _, err := exec.LookPath("vecgrep"); err == nil {
		return true
	}
	return false
}

// totalSize sums the logical size of all stashes.
func (m Model) totalSize() int64 {
	var total int64
	for _, st := range m.stashes {
		if st.Manifest != nil {
			total += st.Manifest.TotalSize
		}
	}
	return total
}

// currentStash returns the stash relevant to actions in the active view.
func (m Model) currentStash() *stash.Stash {
	switch m.activeView {
	case viewDetail:
		return m.selected
	case viewSearch:
		if id := m.currentResultStashID(); id != "" {
			for _, st := range m.stashes {
				if st.Manifest != nil && st.Manifest.ID == id {
					return st
				}
			}
		}
		return nil
	default:
		if m.cursor >= 0 && m.cursor < len(m.stashes) {
			return m.stashes[m.cursor]
		}
	}
	return nil
}

func (m Model) currentResultStashID() string {
	if m.resultIdx >= 0 && m.resultIdx < len(m.searchResults) {
		return m.searchResults[m.resultIdx].StashID
	}
	return ""
}
