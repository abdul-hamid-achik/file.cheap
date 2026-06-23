package studio

import (
	"context"
	"fmt"
	"image"
	"os/exec"
	"sort"
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

// imgCache memoizes rendered half-block art so the View path doesn't re-rasterize
// the image every frame (e.g. while a spinner ticks). It is keyed by the decoded
// image plus the pane size it was rendered for.
type imgCache struct {
	img        image.Image
	cols, rows int
	str        string
}

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
	sortIdx int  // index into stashSortModes
	sortRev bool // reverse the active sort direction

	// live list filter
	filter      string
	filtering   bool // the filter input is focused
	filterInput textinput.Model

	// detail view
	selected   *stash.Stash
	fileIdx    int
	preview    viewport.Model
	previewSeq int // bumped per preview request; a load result is applied only if it still matches (drops out-of-order async loads)

	// image preview: when the selected file is a raster image we decode it once
	// and render it to half-blocks sized to the current pane at View time.
	// previewImg nil means the preview is plain text in the viewport.
	previewImg      image.Image
	previewImgCap   string    // styled caption line (format · dims · size)
	previewImgCache *imgCache // memoized art, keyed by image + pane size

	// video playback: animate a vidtrace frame sequence in the preview pane.
	playing    bool
	playFrames []int // indices (into selectedFiles) of the image frames, in order
	playPos    int   // current position within playFrames
	playFPS    int

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
	seq     int // request token; stale loads (seq != model.previewSeq) are dropped
	stashID string
	title   string
	content string
	img     image.Image // non-nil when the selected file is a decodable raster image
	format  string      // image format ("png", "jpeg", …) when img is set
	size    int64       // logical file size, for the image caption
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

// videoFrameMsg carries one decoded frame during vidtrace playback.
type videoFrameMsg struct {
	pos    int // position within m.playFrames this frame was requested for
	img    image.Image
	format string
	size   int64
	err    error
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

	fi := textinput.New()
	fi.Placeholder = "filter by name / tool / tag…"
	fi.Prompt = "› "
	fi.SetWidth(40)
	fi.SetStyles(styles)

	vp := viewport.New(viewport.WithWidth(60), viewport.WithHeight(16))

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	prog := progress.New(progress.WithDefaultBlend(), progress.WithWidth(34))

	return Model{
		progress:        prog,
		ctx:             ctx,
		stashDir:        stashDir,
		vecgrepPath:     vecgrepPath,
		analyzer:        analyze.NewAnalyzer(stashDir, vecgrepPath).WithEmbedder(emb),
		activeView:      viewList,
		focus:           focusList,
		query:           ti,
		diffInput:       di,
		filterInput:     fi,
		preview:         vp,
		spinner:         sp,
		searchMode:      "auto",
		loading:         true,
		previewImgCache: &imgCache{},
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

	case videoFrameMsg:
		// Stop cleanly if playback was cancelled (a navigation/view change) while
		// this frame was decoding, or the position is stale.
		if !m.playing || m.activeView != viewDetail || msg.pos != m.playPos {
			return m, nil
		}
		if msg.err != nil || msg.img == nil {
			// A frame failed to decode (e.g. content removed mid-play); stop rather
			// than spin re-decoding a missing file.
			m.stopPlayback()
			return m, nil
		}
		m.previewImg = msg.img
		m.previewImgCap = m.playCaption(msg.img, msg.format, msg.size)
		if fi := m.playFrames[msg.pos]; fi >= 0 && fi < len(m.selectedFiles()) {
			m.fileIdx = fi // keep the Files-pane cursor in step with playback
		}
		// Advance to the next frame (looping) and schedule it.
		m.playPos = (msg.pos + 1) % len(m.playFrames)
		return m, m.playFrameCmd(m.playPos)

	case stashesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.stashes = msg.stashes
		m.sortStashes()
		m.errMessage = ""
		m.statusMessage = fmt.Sprintf("%d stash(es)", len(m.stashes))
		if nv := len(m.visible()); m.cursor >= nv {
			m.cursor = clamp(nv-1, 0, nv)
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
		// In detail view, reload the preview so it reflects the now-current stash
		// (e.g. after compress, the file should show "(compressed — restore to view)"
		// instead of the stale uncompressed content).
		if m.activeView == viewDetail && m.selected != nil {
			m.clearPreviewImage()
			return m, m.loadFilePreviewCmd()
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
		// Drop results from a superseded request: when the user navigates files
		// quickly, several decode/read goroutines are in flight and may finish out
		// of order — only the newest request's result should reach the pane.
		if msg.seq != m.previewSeq {
			return m, nil
		}
		// Only detail/search display file/result previews; a load that lands after a
		// switch to diff/timeline/list must not overwrite that pane's content.
		if m.activeView != viewDetail && m.activeView != viewSearch {
			return m, nil
		}
		if msg.err != nil {
			m.clearPreviewImage()
			m.preview.SetContent(warnStyle.Render(msg.err.Error()))
			m.preview.GotoTop()
			return m, nil
		}
		if msg.img != nil {
			m.previewImg = msg.img
			m.previewImgCap = imageCaption(msg.img, msg.format, msg.size)
		} else {
			m.clearPreviewImage()
			m.preview.SetContent(msg.content)
		}
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
		// The image preview re-flows at View time, keyed off the new pane size.
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
		case m.filtering:
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.filter = m.filterInput.Value()
			m.cursor = 0 // refining the filter; reset to the top of the matches
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

	// List filter input intercepts keys until enter (apply) or esc (clear).
	if m.filtering {
		switch key {
		case "enter":
			m.filtering = false
			m.filterInput.Blur()
			return nil, true
		case "esc":
			m.filtering = false
			m.filterInput.Blur()
			m.filterInput.SetValue("")
			m.filter = ""
			m.cursor = 0
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
		// q/esc/enter return to the list; s and ? toggle their own panel closed
		// (pressing s again from Status, or ? again from Help, returns to list).
		switch key {
		case "q", "esc", "?", "s", "enter":
			m.toList()
			return nil, true
		}
	}
	return nil, false
}

// sortMode describes one stash-list ordering, cycled with the "o" key.
type sortMode struct {
	name string // column label it sorts by
	desc bool   // true => ▼ (descending)
	less func(a, b *stash.Stash) bool
}

func stashDisplayName(s *stash.Stash) string {
	if s.Manifest.Name != "" {
		return s.Manifest.Name
	}
	return s.Manifest.ID
}

var stashSortModes = []sortMode{
	{"AGE", true, func(a, b *stash.Stash) bool { return a.Manifest.CreatedAt > b.Manifest.CreatedAt }},
	{"NAME", false, func(a, b *stash.Stash) bool { return stashDisplayName(a) < stashDisplayName(b) }},
	{"TOOL", false, func(a, b *stash.Stash) bool { return a.Manifest.Tool < b.Manifest.Tool }},
	{"FILES", true, func(a, b *stash.Stash) bool { return a.Manifest.FileCount > b.Manifest.FileCount }},
	{"SIZE", true, func(a, b *stash.Stash) bool { return a.Manifest.TotalSize > b.Manifest.TotalSize }},
}

// sortStashes orders the list by the active sort mode (stable, nil-safe),
// reversing the direction when sortRev is set.
func (m *Model) sortStashes() {
	mode := stashSortModes[m.sortIdx%len(stashSortModes)]
	sort.SliceStable(m.stashes, func(i, j int) bool {
		a, b := m.stashes[i], m.stashes[j]
		if a.Manifest == nil || b.Manifest == nil {
			return false
		}
		if m.sortRev {
			a, b = b, a
		}
		return mode.less(a, b)
	})
}

// effectiveSortDesc reports whether the list is currently shown descending.
func (m *Model) effectiveSortDesc() bool {
	return stashSortModes[m.sortIdx%len(stashSortModes)].desc != m.sortRev
}

// visible returns the stashes matching the current filter (case-insensitive
// substring over name / id / tool / tags). Empty filter returns all stashes.
// The list cursor and all list actions operate on this slice.
func (m Model) visible() []*stash.Stash {
	q := strings.ToLower(strings.TrimSpace(m.filter))
	if q == "" {
		return m.stashes
	}
	out := make([]*stash.Stash, 0, len(m.stashes))
	for _, s := range m.stashes {
		if s.Manifest == nil {
			continue
		}
		man := s.Manifest
		hay := strings.ToLower(man.Name + " " + man.ID + " " + man.Tool + " " + strings.Join(man.Tags, " "))
		if strings.Contains(hay, q) {
			out = append(out, s)
		}
	}
	return out
}

func (m *Model) handleListKey(key string) (tea.Cmd, bool) {
	switch key {
	case "q", "esc":
		return tea.Quit, true
	case "j", "down":
		if m.cursor < len(m.visible())-1 {
			m.cursor++
		}
		return nil, true
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return nil, true
	case "enter", "l":
		if len(m.visible()) > 0 {
			m.openDetail()
			return m.loadFilePreviewCmd(), true
		}
		return nil, true
	case "/":
		m.openSearch()
		return nil, true
	case "f":
		m.filtering = true
		m.filterInput.SetValue(m.filter)
		return m.filterInput.Focus(), true
	case "s":
		m.activeView = viewStatus
		return nil, true
	case "?":
		m.activeView = viewHelp
		return nil, true
	case "g", "home":
		m.cursor = 0
		return nil, true
	case "G", "end":
		if n := len(m.visible()); n > 0 {
			m.cursor = n - 1
		}
		return nil, true
	case "pgdown", "ctrl+d":
		if n := len(m.visible()); n > 0 {
			m.cursor = clamp(m.cursor+m.filePageStep(), 0, n-1)
		}
		return nil, true
	case "pgup", "ctrl+u":
		if n := len(m.visible()); n > 0 {
			m.cursor = clamp(m.cursor-m.filePageStep(), 0, n-1)
		}
		return nil, true
	case "R":
		return loadStashesCmd(m.stashDir), true
	case "o":
		if len(m.stashes) > 0 {
			m.sortIdx = (m.sortIdx + 1) % len(stashSortModes)
			m.sortRev = false
			m.sortStashes()
			m.cursor = 0
		}
		return nil, true
	case "O":
		if len(m.stashes) > 0 {
			m.sortRev = !m.sortRev
			m.sortStashes()
			m.cursor = 0
		}
		return nil, true
	case "r":
		return m.restoreCmd(), true
	case "c":
		return m.compressCmd(), true
	case "a":
		return m.indexCmd(), true
	case "x":
		if len(m.visible()) > 0 {
			return m.startDiffPrompt(), true
		}
		return nil, true
	case "d":
		if len(m.visible()) > 0 {
			m.confirm = confirmDrop
		}
		return nil, true
	}
	return nil, false
}

func (m *Model) handleDetailKey(key string) (tea.Cmd, bool) {
	files := m.selectedFiles()
	// Any key other than the play toggle hands control back to the user and stops
	// playback (an in-flight frame is then dropped by the stale-position guard).
	if key != "p" && key != " " && key != "space" {
		m.stopPlayback()
	}
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
		if m.focus == focusFiles {
			return m.moveFileCursor(m.filePageStep()), true
		}
		m.preview.HalfPageDown()
		return nil, true
	case "pgup", "ctrl+u":
		if m.focus == focusFiles {
			return m.moveFileCursor(-m.filePageStep()), true
		}
		m.preview.HalfPageUp()
		return nil, true
	case "g", "home":
		if m.focus == focusFiles {
			return m.setFileCursor(0), true
		}
		m.preview.GotoTop()
		return nil, true
	case "G", "end":
		if m.focus == focusFiles {
			return m.setFileCursor(len(files) - 1), true
		}
		m.preview.GotoBottom()
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
	case "p", " ", "space":
		// Play/pause the frame sequence (vidtrace bundles, or any stash with ≥2
		// images). stopPlayback already ran above for non-toggle keys.
		if m.playing {
			m.stopPlayback()
			return nil, true
		}
		return m.startPlayback(), true
	case "r":
		return m.restoreCmd(), true
	case "c":
		return m.compressCmd(), true
	case "a":
		return m.indexCmd(), true
	case "x":
		return m.startDiffPrompt(), true
	case "d":
		// Only drop from the files pane; in the preview pane `d` is the
		// half-page-down pager key and must not trigger a destructive drop.
		if m.focus == focusPreview {
			m.preview.HalfPageDown()
			return nil, true
		}
		m.confirm = confirmDrop
		return nil, true
	case "u":
		if m.focus == focusPreview {
			m.preview.HalfPageUp()
		}
		return nil, true
	}
	return nil, false
}

// startDiffPrompt focuses the diff path input, prefilled with the stash source.
func (m *Model) startDiffPrompt() tea.Cmd {
	m.diffPrompting = true
	m.statusMessage = ""
	if st := m.currentStash(); st != nil && st.Manifest != nil {
		// Anchor the diff to a stash regardless of entry point (list or detail),
		// so the diff panel is titled and `esc` returns to that stash's detail.
		m.selected = st
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
			// Re-enter the stash detail cleanly: the diff left its own text (and
			// possibly a stale image) in the preview and m.fileIdx may be left over
			// from an earlier detail session, so reset and reload like openDetail.
			m.activeView = viewDetail
			m.focus = focusFiles
			m.fileIdx = 0
			m.clearPreviewImage()
			return m.loadFilePreviewCmd(), true
		}
		m.toList()
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
	case "g", "home":
		m.preview.GotoTop()
		return nil, true
	case "G", "end":
		m.preview.GotoBottom()
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
	case "g", "home":
		m.preview.GotoTop()
		return nil, true
	case "G", "end":
		m.preview.GotoBottom()
		return nil, true
	case "?":
		m.activeView = viewHelp
		return nil, true
	}
	return nil, false
}

func (m *Model) handleSearchKey(key string) (tea.Cmd, bool) {
	switch key {
	case "q", "esc", "h":
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
	case "g", "home":
		if m.focus == focusPreview {
			m.preview.GotoTop()
			return nil, true
		}
		if len(m.searchResults) > 0 { // results pane: jump to the first match
			m.resultIdx = 0
			return m.loadResultPreviewCmd(), true
		}
		return nil, true
	case "G", "end":
		if m.focus == focusPreview {
			m.preview.GotoBottom()
			return nil, true
		}
		if n := len(m.searchResults); n > 0 { // results pane: jump to the last match
			m.resultIdx = n - 1
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
	m.clearPreviewImage()
	m.stopPlayback()
}

func (m *Model) openDetail() {
	vis := m.visible()
	if m.cursor < 0 || m.cursor >= len(vis) {
		return
	}
	m.selected = vis[m.cursor]
	m.activeView = viewDetail
	m.focus = focusFiles
	m.fileIdx = 0
}

func (m *Model) openSearch() {
	m.activeView = viewSearch
	m.focus = focusQuery
	m.query.Focus()
	// Drop any image/text decoded for the detail pane; otherwise renderPreview
	// (which also renders previewImg under viewSearch, and otherwise the viewport's
	// last content) would show the stale detail preview in the search Preview pane
	// until a result loads — and never, if the query returns zero results.
	m.clearPreviewImage()
	m.preview.SetContent("")
}

// cycleSearchFocus advances query -> results -> preview -> query. With no results
// there is nothing to navigate to, so it keeps (or restores) focus on the query
// rather than stranding it on an empty results/preview pane.
func (m *Model) cycleSearchFocus() {
	if len(m.searchResults) == 0 {
		if m.focus != focusQuery {
			m.focus = focusQuery
			m.query.Focus()
		}
		return
	}
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

	// Generous default preview height; the view renderers refine it per-layout
	// each frame so the preview fills the available space. This value mainly
	// drives the scroll amount for page-up/down in the Update path.
	previewHeight := clamp(m.height-8, 6, m.height)
	previewWidth := clamp(m.width/2-4, 30, m.width-4)
	if m.width < 96 {
		previewWidth = clamp(m.width-4, 30, m.width-4)
		previewHeight = clamp(m.height/2-6, 5, m.height)
	}
	m.preview.SetWidth(previewWidth)
	m.preview.SetHeight(previewHeight)
}

// clearPreviewImage drops any decoded image so the preview falls back to text.
func (m *Model) clearPreviewImage() {
	m.previewImg = nil
	m.previewImgCap = ""
	if m.previewImgCache != nil {
		*m.previewImgCache = imgCache{}
	}
}

// setFileCursor moves the detail-view file selection to idx (clamped) and loads
// that file's preview. It returns nil when the position is unchanged.
func (m *Model) setFileCursor(idx int) tea.Cmd {
	files := m.selectedFiles()
	if len(files) == 0 {
		return nil
	}
	ni := clamp(idx, 0, len(files)-1)
	if ni == m.fileIdx {
		return nil
	}
	m.fileIdx = ni
	return m.loadFilePreviewCmd()
}

// moveFileCursor moves the file selection by delta rows.
func (m *Model) moveFileCursor(delta int) tea.Cmd {
	return m.setFileCursor(m.fileIdx + delta)
}

// filePageStep is the row jump for page-up/down in the Files pane, scaled to the
// terminal height so it advances roughly a screenful at a time.
func (m Model) filePageStep() int {
	return clamp(m.height/3, 1, 40)
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
		vis := m.visible()
		if m.cursor >= 0 && m.cursor < len(vis) {
			return vis[m.cursor]
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
