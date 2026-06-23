package studio

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

func (m Model) View() tea.View {
	content := m.render()
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) render() string {
	if m.width == 0 {
		return "fcheap studio\n\nloading…"
	}

	header := lipgloss.NewStyle().MaxWidth(m.width).Render(m.renderHeader())
	// Wrap a long footer to the terminal width (at the "  " gaps between hints)
	// instead of overflowing, which would force the whole UI wider than the screen.
	footer := lipgloss.NewStyle().Width(m.width).Render(m.renderFooter())
	headerH := lipgloss.Height(header)
	footerH := lipgloss.Height(footer)
	// Too short to hold header + a body row + footer: show just the footer (it
	// carries the drop-confirm and diff prompts), clipped to the terminal.
	if m.height < headerH+footerH+1 {
		return lipgloss.NewStyle().Height(m.height).MaxHeight(m.height).Render(footer)
	}
	// The body fills every row between the header and the footer, so views use
	// the full terminal height and the footer pins to the bottom.
	bodyH := m.height - headerH - footerH
	if bodyH < 1 {
		bodyH = 1
	}

	var body string
	switch m.activeView {
	case viewDetail:
		body = m.renderDetail(bodyH)
	case viewSearch:
		body = m.renderSearch(bodyH)
	case viewTimeline:
		body = m.renderTimeline(bodyH)
	case viewDiff:
		body = m.renderDiff(bodyH)
	case viewStatus:
		body = m.renderStatus(bodyH)
	case viewHelp:
		body = m.renderHelp(bodyH)
	default:
		body = m.renderList(bodyH)
	}
	// Pad (or clip) the body to exactly bodyH rows so the footer sits at the bottom.
	body = lipgloss.NewStyle().Height(bodyH).MaxHeight(bodyH).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// --- header ---

func (m Model) renderHeader() string {
	left := brandStyle.Render("fcheap") + " " + titleStyle.Render("studio")
	summary := fmt.Sprintf("%d stash(es)", len(m.stashes))
	if len(m.stashes) > 0 {
		var total int64
		for _, s := range m.stashes {
			if s.Manifest != nil {
				total += s.Manifest.TotalSize
			}
		}
		summary = fmt.Sprintf("%d stashes · %s", len(m.stashes), formatSize(total))
	}
	right := mutedStyle.Render(summary)
	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad > 1 {
		return left + strings.Repeat(" ", pad) + right
	}
	return left
}

// --- list view ---

func (m Model) renderList(h int) string {
	if m.loading {
		return m.renderPanelH("Stashes", m.spinner.View()+" "+mutedStyle.Render("loading stashes…"), m.width-2, h, true)
	}
	if len(m.stashes) == 0 {
		hint := lipgloss.JoinVertical(lipgloss.Left,
			mutedStyle.Render("No stashes yet."),
			"",
			dimStyle.Render("Create one with:"),
			keyStyle.Render("  fcheap save <path>"),
		)
		return m.renderPanelH("Stashes", hint, m.width-2, h, true)
	}

	mode := stashSortModes[m.sortIdx%len(stashSortModes)]
	arrow := "▲"
	if m.effectiveSortDesc() {
		arrow = "▼"
	}
	title := fmt.Sprintf("Stashes · %s %s", mode.name, arrow)
	// m.width-6 is the panel interior (m.width-2 box minus the 4-col border+padding).
	rows := m.renderStashRows(m.width-6, h)
	return m.renderPanelH(title, rows, m.width-2, h, true)
}

// stash-list column widths (the NAME column flexes to fill the rest).
const (
	colTool  = 12
	colFiles = 6
	colSize  = 10
	colAge   = 11
	colChips = 22 // reserved on the right for compression + secrets chips (+ gaps)
)

// nameColWidth flexes the NAME column to use the available horizontal space.
func nameColWidth(width int) int {
	fixed := 2 + colTool + colFiles + colSize + colAge + 4*2 + colChips
	return clamp(width-fixed, 16, 72)
}

func (m Model) renderStashRows(width, h int) string {
	vis := m.visible()
	nameW := nameColWidth(width)
	var b strings.Builder
	// clip truncates a line to the panel interior (ANSI-aware) so chip-bearing or
	// long lines never wrap and corrupt the columnar layout at narrow widths.
	clip := func(s string) string { return lipgloss.NewStyle().MaxWidth(width).Render(s) }

	bodyRows := panelBodyHeight(h) - 1 // minus the column-header row

	// Filter line, when filtering or a filter is applied.
	if m.filtering || m.filter != "" {
		bodyRows--
		if m.filtering {
			b.WriteString(clip(mutedStyle.Render("filter: ") + m.filterInput.View()))
		} else {
			b.WriteString(clip(mutedStyle.Render("filter: ") + inkStyle.Render(m.filter) +
				mutedStyle.Render(fmt.Sprintf("   %d of %d  ·  f edit · esc clear", len(vis), len(m.stashes)))))
		}
		b.WriteString("\n")
	}

	active := stashSortModes[m.sortIdx%len(stashSortModes)].name
	hcell := func(label string, w int, right bool) string {
		s := fmt.Sprintf("%-*s", w, label)
		if right {
			s = fmt.Sprintf("%*s", w, label)
		}
		if label == active {
			return colHeaderActiveStyle.Render(s)
		}
		return colHeaderStyle.Render(s)
	}
	b.WriteString(clip("  " + hcell("NAME", nameW, false) + "  " + hcell("TOOL", colTool, false) +
		"  " + hcell("FILES", colFiles, true) + "  " + hcell("SIZE", colSize, true) +
		"  " + hcell("AGE", colAge, false)))
	b.WriteString("\n")

	if len(vis) == 0 {
		b.WriteString(mutedStyle.Render("  no stashes match the filter"))
		return b.String()
	}

	// Reserve a row for the "… N more" indicator when the list overflows, so the
	// panel's bottom border isn't pushed out and clipped.
	if len(vis) > bodyRows {
		bodyRows--
	}

	maxRows := clamp(bodyRows, 1, len(vis))
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end := clamp(start+maxRows, 0, len(vis))

	for i := start; i < end; i++ {
		selected := i == m.cursor && m.activeView == viewList
		row := clip(m.renderStashRow(vis[i], i == m.cursor, nameW, selected))
		if selected {
			b.WriteString(selectedRowStyle.Width(width).Render(row))
		} else {
			b.WriteString(row)
		}
		b.WriteString("\n")
	}
	if end < len(vis) {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  … %d more", len(vis)-end)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderStashRow(st *stash.Stash, cursor bool, nameW int, selected bool) string {
	man := st.Manifest
	marker := "  "
	if cursor {
		marker = "▸ "
	}
	name := man.ID
	if man.Name != "" {
		name = man.Name
	}

	nameCol := fmt.Sprintf("%-*s", nameW, truncate(name, nameW))
	toolCol := fmt.Sprintf("%-*s", colTool, truncate(man.Tool, colTool))
	filesCol := fmt.Sprintf("%*d", colFiles, man.FileCount)
	sizeCol := fmt.Sprintf("%*s", colSize, formatSize(man.TotalSize))
	ageCol := fmt.Sprintf("%-*s", colAge, truncate(relTime(man.CreatedAt), colAge))
	// Color the columns only on unselected rows — the selection style owns the
	// look of the highlighted row.
	if !selected {
		toolCol = toolStyle(man.Tool).Render(toolCol)
		sizeCol = dimStyle.Render(sizeCol)
		ageCol = mutedStyle.Render(ageCol)
	}

	row := marker + nameCol + "  " + toolCol + "  " + filesCol + "  " + sizeCol + "  " + ageCol
	if man.Compression != "" {
		row += "  " + zstChipStyle.Render(compLabel(man.Compression))
	}
	if man.Custom["secrets_found"] != "" {
		row += "  " + warnChipStyle.Render("⚠ secrets")
	}
	return row
}

// --- detail view ---

func (m Model) renderDetail(h int) string {
	if m.selected == nil || m.selected.Manifest == nil {
		return m.renderPanelH("Detail", mutedStyle.Render("no stash selected"), m.width-2, h, true)
	}
	man := m.selected.Manifest

	var info strings.Builder

	detailSection(&info, "IDENTITY")
	kvLine(&info, "ID", dimStyle.Render(man.ID))
	if man.Name != "" {
		kvLine(&info, "Name", inkStyle.Render(man.Name))
	}

	detailSection(&info, "PROVENANCE")
	if man.SourcePath != "" {
		kvLine(&info, "Source", inkStyle.Render(man.SourcePath))
	}
	if man.Tool != "" {
		kvLine(&info, "Tool", toolStyle(man.Tool).Render(man.Tool))
	}
	created := inkStyle.Render(relTime(man.CreatedAt))
	if abs := absTime(man.CreatedAt); abs != "" {
		created += dimStyle.Render("  ·  " + abs)
	}
	kvLine(&info, "Created", created)
	if man.BundleType != "" && man.BundleType != "generic" {
		kvLine(&info, "Bundle", bundleChipStyle(man.BundleType).Render(man.BundleType))
	}
	if v := man.VideoSummary(); v != "" {
		kvLine(&info, "Video", inkStyle.Render(v))
	}

	detailSection(&info, "CONTENT")
	kvLine(&info, "Files", inkStyle.Render(fmt.Sprintf("%d", man.FileCount)))
	kvLine(&info, "Size", inkStyle.Render(formatSize(man.TotalSize)))
	kvLine(&info, "Hash", dimStyle.Render(shortHash(man.ContentHash)))
	if len(man.Tags) > 0 {
		chips := make([]string, 0, len(man.Tags))
		for _, t := range man.Tags {
			chips = append(chips, tagChipStyle.Render(t))
		}
		kvLine(&info, "Tags", strings.Join(chips, " "))
	}
	if c := man.Custom["secrets_found"]; c != "" {
		kvLine(&info, "Secrets", warnChipStyle.Render("⚠ "+c+" potential"))
	}

	detailSection(&info, "STORAGE")
	if man.Compression != "" {
		stored := inkStyle.Render(fmt.Sprintf("%s · %s", compLabel(man.Compression), formatSize(man.CompressedSize)))
		if man.TotalSize > 0 && man.CompressedSize > 0 && man.CompressedSize < man.TotalSize {
			pct := 100 - (man.CompressedSize*100)/man.TotalSize
			stored += dimStyle.Render(fmt.Sprintf("  ·  %d%% smaller", pct))
		}
		kvLine(&info, "Stored", stored)
	} else {
		kvLine(&info, "Stored", mutedStyle.Render("uncompressed"))
	}
	if man.Custom["indexed"] == "true" {
		idx := goodStyle.Render("✓ analyzed")
		if n := man.Custom["indexed_files"]; n != "" {
			idx += dimStyle.Render("  (" + n + " docs)")
		}
		kvLine(&info, "Indexed", idx)
	} else {
		kvLine(&info, "Indexed", mutedStyle.Render("— not indexed"))
	}

	filesTitle := fmt.Sprintf("Files (%d)", man.FileCount)

	infoStr := info.String()
	provNatural := lipgloss.Height(infoStr) + 3 // content + border(2) + title(1)

	if m.width >= 96 {
		leftW := clamp(m.width/2-2, 30, m.width-4)
		rightW := m.width - leftW - 4
		// Right preview fills the full body height; the left column stacks
		// Provenance (capped so Files keeps room) above Files (the remainder).
		// Size the viewport to the actual right-panel interior (panel width minus
		// border+padding) so both text and image art fit exactly at any width.
		m.preview.SetWidth(rightW - 4)
		m.preview.SetHeight(panelBodyHeight(h))
		right := m.renderPanelH(m.previewTitle(), m.renderPreview(), rightW, h, m.focus == focusPreview)
		provH := clamp(provNatural, 6, h-6)
		filesH := h - provH
		left := lipgloss.JoinVertical(lipgloss.Left,
			m.renderPanelClip("Provenance", infoStr, leftW, provH, false),
			m.renderPanelClip(filesTitle, m.renderFileTree(panelBodyHeight(filesH), leftW-4), leftW, filesH, m.focus == focusFiles),
		)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	}

	// Stacked: Provenance, Files, Preview — each capped so all three stay visible.
	provH := clamp(provNatural, 5, h-9)
	naturalFiles := len(m.selectedFiles()) + 1 // file rows + a possible "… more" line
	filesH := clamp(naturalFiles+3, 4, h-provH-4)
	previewH := h - provH - filesH
	if previewH < 3 {
		previewH = 3
	}
	// Full-width preview panel (m.width-2): interior is m.width-6.
	m.preview.SetWidth(clamp(m.width-6, 20, m.width))
	m.preview.SetHeight(panelBodyHeight(previewH))
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderPanelClip("Provenance", infoStr, m.width-2, provH, false),
		m.renderPanelClip(filesTitle, m.renderFileTree(panelBodyHeight(filesH), m.width-6), m.width-2, filesH, m.focus == focusFiles),
		m.renderPanelH(m.previewTitle(), m.renderPreview(), m.width-2, previewH, m.focus == focusPreview),
	)
}

// renderFileTree renders the file list windowed to exactly bodyRows visible rows
// (the panel's interior height) so the cursor always stays on screen. Sizing the
// scroll window to the real panel height — not the whole terminal — is what makes
// the list scroll instead of appearing frozen once the cursor passes the fold.
func (m Model) renderFileTree(bodyRows, interior int) string {
	files := m.selectedFiles()
	if len(files) == 0 {
		return mutedStyle.Render("(no files)")
	}
	if bodyRows < 1 {
		bodyRows = 1
	}
	if interior < 12 {
		interior = 12
	}
	// clip guarantees a row never wraps the panel (MaxWidth truncates ANSI-aware,
	// rather than wrapping, which would corrupt the columnar layout, double the row
	// count vs. the scroll math, and push the bottom border off-screen). pathW flexes
	// the name column to the panel width so columns stay aligned at any size.
	clip := func(s string) string { return lipgloss.NewStyle().MaxWidth(interior).Render(s) }
	const markerW, sizeW = 2, 8
	pathW := interior - markerW - sizeW - 1
	if pathW < 8 {
		pathW = 8
	}
	// Reserve a row for the "… N more" indicator when the list overflows so it
	// doesn't hide the last file.
	maxRows := bodyRows
	if len(files) > bodyRows && maxRows > 1 {
		maxRows--
	}
	start := 0
	if m.fileIdx >= maxRows {
		start = m.fileIdx - maxRows + 1
	}
	end := clamp(start+maxRows, 0, len(files))

	var b strings.Builder
	for i := start; i < end; i++ {
		marker := "  "
		if i == m.fileIdx {
			marker = "▸ "
		}
		line := fmt.Sprintf("%s%-*s %*s", marker, pathW, truncate(files[i].Path, pathW), sizeW, formatSize(files[i].Size))
		if i == m.fileIdx && m.focus == focusFiles {
			b.WriteString(clip(selectedRowStyle.Render(line)))
		} else {
			b.WriteString(clip(line))
		}
		b.WriteString("\n")
	}
	if end < len(files) {
		b.WriteString(clip(mutedStyle.Render(fmt.Sprintf("  … %d more", len(files)-end))))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) previewTitle() string {
	if m.activeView == viewSearch {
		return "Preview"
	}
	files := m.selectedFiles()
	if m.fileIdx >= 0 && m.fileIdx < len(files) {
		return "Preview · " + truncate(files[m.fileIdx].Path, 40)
	}
	return "Preview"
}

func (m Model) renderPreview() string {
	// Image previews bypass the viewport (the art is fit to the pane, nothing to
	// scroll). Only the detail and search panes load images, so gate on those.
	if m.previewImg != nil && (m.activeView == viewDetail || m.activeView == viewSearch) {
		return m.imageArt()
	}
	return m.preview.View()
}

// imageArt renders the loaded image to half-block art sized to the current preview
// pane. It memoizes the result (keyed by image + pane size) so repeated frames —
// e.g. while a spinner ticks — don't re-rasterize. The render funcs set the
// viewport width to the panel interior, so cols is exactly the available width and
// the un-wrappable block art never wraps.
func (m Model) imageArt() string {
	cols := m.preview.Width()
	rows := m.preview.Height()
	if cols < 1 {
		cols = 38
	}
	if rows < 4 {
		rows = 4
	}
	if c := m.previewImgCache; c != nil && c.img == m.previewImg && c.cols == cols && c.rows == rows {
		return c.str
	}
	art := renderImageBlocks(m.previewImg, cols, rows-2)
	if m.previewImgCap != "" {
		art += "\n\n" + m.previewImgCap
	}
	if c := m.previewImgCache; c != nil {
		*c = imgCache{img: m.previewImg, cols: cols, rows: rows, str: art}
	}
	return art
}

// --- search view ---

func (m Model) renderSearch(h int) string {
	queryPanel := m.sizePanel("Search · mode "+m.searchMode, m.query.View(), m.width-2, m.focus == focusQuery)
	restH := h - lipgloss.Height(queryPanel)
	if restH < 4 {
		restH = 4
	}

	resultsTitle := "Results"
	if len(m.searchResults) > 0 {
		resultsTitle = fmt.Sprintf("Results %d/%d", m.resultIdx+1, len(m.searchResults))
		if src := m.searchResults[0].Source; src != "" {
			resultsTitle += " · " + src // actual mode used (keyword/semantic/hybrid)
		}
	}

	if m.width >= 96 {
		leftW := clamp(m.width/2-2, 30, m.width-4)
		rightW := m.width - leftW - 4
		m.preview.SetWidth(rightW - 4) // panel interior
		m.preview.SetHeight(panelBodyHeight(restH))
		left := m.renderPanelH(resultsTitle, m.renderSearchResults(panelBodyHeight(restH)), leftW, restH, m.focus == focusResults)
		right := m.renderPanelH("Preview", m.renderPreview(), rightW, restH, m.focus == focusPreview)
		return lipgloss.JoinVertical(lipgloss.Left,
			queryPanel,
			lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right),
		)
	}
	// Stacked: split the remaining height between results and preview.
	resultsH := restH / 2
	previewH := restH - resultsH
	m.preview.SetWidth(clamp(m.width-6, 20, m.width)) // panel interior
	m.preview.SetHeight(panelBodyHeight(previewH))
	return lipgloss.JoinVertical(lipgloss.Left,
		queryPanel,
		m.renderPanelH(resultsTitle, m.renderSearchResults(panelBodyHeight(resultsH)), m.width-2, resultsH, m.focus == focusResults),
		m.renderPanelH("Preview", m.renderPreview(), m.width-2, previewH, m.focus == focusPreview),
	)
}

func (m Model) renderSearchResults(bodyRows int) string {
	if m.searching {
		return m.spinner.View() + " " + mutedStyle.Render("searching…")
	}
	if len(m.searchResults) == 0 {
		if m.lastQuery == "" {
			return mutedStyle.Render("Type a query and press enter.\nIndex stashes first with 'a'.")
		}
		return mutedStyle.Render("No matches.")
	}
	width := clamp(m.width/2-6, 30, m.width-6)
	if m.width < 96 {
		width = m.width - 6
	}
	maxRows := clamp(bodyRows, 1, len(m.searchResults))
	start := 0
	if m.resultIdx >= maxRows {
		start = m.resultIdx - maxRows + 1
	}
	end := clamp(start+maxRows, 0, len(m.searchResults))

	top := m.searchResults[0].Score // results are sorted by score desc
	var b strings.Builder
	for i := start; i < end; i++ {
		r := m.searchResults[i]
		marker := "  "
		if i == m.resultIdx {
			marker = "▸ "
		}
		file := r.File
		if file == "" {
			file = "(stash)"
		}
		head := fmt.Sprintf("%s%s › %s  ", marker, truncate(r.StashID, 16), truncate(file, 28))
		score := scoreStyle(r.Score, top).Render(fmt.Sprintf("%.2f", r.Score))
		line := truncate(head, width-6) + score
		if i == m.resultIdx && m.focus == focusResults {
			b.WriteString(selectedRowStyle.Width(width).Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// --- timeline view (vidtrace bundles) ---

func (m Model) renderTimeline(h int) string {
	title := "Timeline"
	if m.selected != nil && m.selected.Manifest != nil && m.selected.Manifest.Name != "" {
		title = "Timeline · " + m.selected.Manifest.Name
	}
	// Full-width panel: fill its interior (the side-by-side widths from resize()
	// would otherwise leave the right half blank and wrap lines early).
	m.preview.SetWidth(clamp(m.width-6, 20, m.width))
	m.preview.SetHeight(panelBodyHeight(h))
	return m.renderPanelH(title, m.preview.View(), m.width-2, h, true)
}

// --- diff view ---

func (m Model) renderDiff(h int) string {
	title := "Diff"
	if m.selected != nil && m.selected.Manifest != nil && m.selected.Manifest.Name != "" {
		title = "Diff · " + m.selected.Manifest.Name
	}
	// Full-width panel: fill its interior so diff lines use the whole width.
	m.preview.SetWidth(clamp(m.width-6, 20, m.width))
	m.preview.SetHeight(panelBodyHeight(h))
	return m.renderPanelH(title, m.preview.View(), m.width-2, h, true)
}

// --- status view ---

func (m Model) renderStatus(h int) string {
	vec := "not available"
	vecStyle := warnStyle
	if m.vecgrepAvailable() {
		vec = "available"
		vecStyle = goodStyle
	}

	indexed, compressed := 0, 0
	for _, st := range m.stashes {
		if st.Manifest == nil {
			continue
		}
		if st.Manifest.Custom["indexed"] == "true" {
			indexed++
		}
		if st.Manifest.Compression != "" {
			compressed++
		}
	}
	hasDB, hasVec := m.indexFiles()

	lines := []string{
		dimStyle.Render("Stash dir:    ") + inkStyle.Render(m.stashDir),
		dimStyle.Render("Stashes:      ") + inkStyle.Render(fmt.Sprintf("%d", len(m.stashes))),
		dimStyle.Render("Total size:   ") + inkStyle.Render(formatSize(m.totalSize())),
		dimStyle.Render("Indexed:      ") + inkStyle.Render(fmt.Sprintf("%d / %d", indexed, len(m.stashes))),
		dimStyle.Render("Compressed:   ") + inkStyle.Render(fmt.Sprintf("%d / %d", compressed, len(m.stashes))),
		dimStyle.Render("Metadata idx: ") + presence(hasDB),
		dimStyle.Render("Search idx:   ") + presence(hasVec),
		dimStyle.Render("Vecgrep:      ") + vecStyle.Render(vec),
	}
	if m.vecgrepPath != "" {
		lines = append(lines, dimStyle.Render("Vecgrep path: ")+mutedStyle.Render(m.vecgrepPath))
	}
	return m.renderPanelH("Status", strings.Join(lines, "\n"), m.width-2, h, true)
}

// presence renders a green "present" / dim "—" indicator.
func presence(ok bool) string {
	if ok {
		return goodStyle.Render("present")
	}
	return mutedStyle.Render("—")
}

// --- help view ---

func (m Model) renderHelp(h int) string {
	sections := []string{
		titleStyle.Render("Navigation"),
		help("j / k  ↑ / ↓", "move cursor"),
		help("g / G", "first / last"),
		help("pgdn / pgup", "page down / up"),
		help("enter / l", "open stash detail"),
		help("esc / h", "back to list"),
		help("tab", "cycle pane focus"),
		help("/", "search"),
		help("s", "status"),
		help("?", "this help"),
		help("q / esc", "quit (or back)"),
		help("ctrl+c", "force quit"),
		"",
		titleStyle.Render("Actions"),
		help("r", "restore to temp dir"),
		help("c", "compress (zstd)"),
		help("a", "analyze / index for search"),
		help("x", "diff against a directory"),
		help("t", "vidtrace timeline (bundles)"),
		help("d", "drop (confirm y/n) — list / files pane"),
		help("f", "filter list (name / tool / tag)"),
		help("o / O", "cycle sort / reverse direction"),
		help("R", "refresh stash list"),
		"",
		titleStyle.Render("Files pane (detail, when focused)"),
		help("j / k", "select prev / next file"),
		help("pgdn / pgup", "jump a page of files"),
		help("ctrl+d / ctrl+u", "jump a page of files"),
		help("g / G", "first / last file"),
		help("images", "render inline (png/jpg/gif)"),
		help("p / space", "play frame sequence (video)"),
		"",
		titleStyle.Render("Preview pane (when focused)"),
		help("j / k", "scroll one line"),
		help("d / u", "half-page down / up"),
		help("pgdn / pgup", "half-page down / up"),
		help("ctrl+d / ctrl+u", "half-page down / up"),
		help("g / G", "top / bottom"),
		"",
		titleStyle.Render("Search"),
		help("/", "focus query input"),
		help("enter", "run search"),
		help("m", "cycle mode — auto/keyword/semantic/hybrid (results/preview pane)"),
		help("tab", "query ↔ results ↔ preview"),
	}
	return m.renderPanelH("Help", strings.Join(sections, "\n"), m.width-2, h, true)
}

func help(key, action string) string {
	return keyStyle.Render(fmt.Sprintf("  %-16s", key)) + hintStyle.Render(action)
}

// --- footer ---

func (m Model) renderFooter() string {
	if m.diffPrompting {
		return warnStyle.Render("diff against: ") + m.diffInput.View()
	}
	if m.confirm == confirmDrop {
		st := m.currentStash()
		id := "stash"
		if st != nil && st.Manifest != nil {
			id = st.Manifest.ID
		}
		return warnStyle.Render(fmt.Sprintf("drop %s? ", id)) + keyStyle.Render("y") + hintStyle.Render("/") + keyStyle.Render("n")
	}

	var parts []string
	switch {
	case m.errMessage != "":
		parts = append(parts, errorStyle.Render("✗ "+m.errMessage))
	case m.indexing && m.indexTotal > 0:
		parts = append(parts, m.progress.View()+" "+mutedStyle.Render(fmt.Sprintf("indexing %d/%d", m.indexDone, m.indexTotal)))
	case m.busy():
		msg := m.statusMessage
		if msg == "" {
			msg = "working…"
		}
		parts = append(parts, m.spinner.View()+" "+mutedStyle.Render(msg))
	case m.statusMessage != "":
		parts = append(parts, mutedStyle.Render(m.statusMessage))
	}
	parts = append(parts, m.contextHints())
	return strings.Join(parts, "  ·  ")
}

func (m Model) contextHints() string {
	switch m.activeView {
	case viewDetail:
		hints := []string{keyHint("tab", "pane")}
		if m.selected != nil && m.selected.Manifest != nil && m.selected.Manifest.BundleType == "vidtrace" {
			hints = append(hints, keyHint("t", "timeline"))
		}
		if m.playing {
			hints = append(hints, keyHint("p", "stop ▶"))
		} else if m.hasFrames() {
			hints = append(hints, keyHint("p", "play ▶"))
		}
		hints = append(hints,
			keyHint("r", "restore"), keyHint("c", "compress"),
			keyHint("a", "index"), keyHint("x", "diff"),
		)
		// `d` is "drop" only off the preview pane; there it's the half-page pager,
		// so don't advertise a destructive action that won't happen.
		if m.focus == focusPreview {
			hints = append(hints, keyHint("d/u", "page"))
		} else {
			hints = append(hints, keyHint("d", "drop"))
		}
		hints = append(hints, keyHint("esc", "back"))
		return strings.Join(hints, "  ")
	case viewTimeline, viewDiff:
		return strings.Join([]string{
			keyHint("j/k", "scroll"), keyHint("g/G", "top/btm"), keyHint("esc", "back"), keyHint("q", "quit"),
		}, "  ")
	case viewSearch:
		hints := []string{keyHint("/", "query"), keyHint("enter", "search"), keyHint("tab", "focus")}
		// `m` (cycle mode) is live only once focus leaves the query input; while
		// typing it would just insert an 'm', so don't advertise it there.
		if m.focus != focusQuery {
			hints = append(hints, keyHint("m", "mode"))
		}
		hints = append(hints, keyHint("esc", "back"))
		return strings.Join(hints, "  ")
	case viewStatus, viewHelp:
		return keyHint("esc", "back")
	default:
		return strings.Join([]string{
			keyHint("enter", "open"), keyHint("/", "search"), keyHint("f", "filter"),
			keyHint("o", "sort"), keyHint("r", "restore"), keyHint("c", "compress"),
			keyHint("a", "index"), keyHint("x", "diff"), keyHint("d", "drop"),
			keyHint("R", "refresh"), keyHint("s", "status"), keyHint("?", "help"), keyHint("q", "quit"),
		}, "  ")
	}
}

// --- panels ---

// renderPanel renders a bordered, titled panel. Width 0 means "fit content".
func (m Model) renderPanel(title, body string, width int, focused bool) string {
	titleStyleFn := panelTitleStyle
	style := panelStyle
	if focused {
		titleStyleFn = activePanelTitleStyle
		style = focusedPanelStyle
	}
	content := lipgloss.JoinVertical(lipgloss.Left, titleStyleFn.Render(title), body)
	if width > 0 {
		return style.Width(width).Render(content)
	}
	return style.Render(content)
}

// sizePanel is renderPanel with an explicit width (>= a sane minimum).
func (m Model) sizePanel(title, body string, width int, focused bool) string {
	if width < 20 {
		width = 20
	}
	return m.renderPanel(title, body, width, focused)
}

// renderPanelH is sizePanel with a fixed total height (totalH rows including the
// border), so the box fills the space rather than shrinking to its content.
func (m Model) renderPanelH(title, body string, width, totalH int, focused bool) string {
	if width < 20 {
		width = 20
	}
	titleStyleFn := panelTitleStyle
	style := panelStyle
	if focused {
		titleStyleFn = activePanelTitleStyle
		style = focusedPanelStyle
	}
	// Keep the title to one row: a title wider than the interior (border+padding =
	// 4 cols) would wrap, making the box taller than totalH so its bottom border
	// gets clipped. This single guard protects every titled panel.
	if interior := width - 4; interior > 0 {
		title = truncate(title, interior)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, titleStyleFn.Render(title), body)
	style = style.Width(width)
	if totalH > 2 {
		// Height() sizes the whole bordered block, so pass the full target.
		style = style.Height(totalH)
	}
	return style.Render(content)
}

// renderPanelClip is renderPanelH but clips the body to the panel's content
// height first (lipgloss Height only grows a box, so tall content must be
// trimmed to keep the panel at totalH).
func (m Model) renderPanelClip(title, body string, width, totalH int, focused bool) string {
	body = lipgloss.NewStyle().MaxHeight(panelBodyHeight(totalH)).Render(body)
	return m.renderPanelH(title, body, width, totalH, focused)
}

// panelBodyHeight returns the rows available for a panel's body given the
// panel's total height (subtracting the border and the title row).
func panelBodyHeight(totalH int) int {
	h := totalH - 3
	if h < 1 {
		h = 1
	}
	return h
}

// --- formatting helpers ---

// detailSection writes a colored group header (with a blank line before it,
// except the first) into the provenance pane.
func detailSection(b *strings.Builder, title string) {
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	b.WriteString(sectionStyle.Render(title) + "\n")
}

// kvLine writes an aligned "  label  value" row; value is already styled.
func kvLine(b *strings.Builder, label, value string) {
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %-9s", label)) + value + "\n")
}

// absTime formats an RFC3339 timestamp as a readable absolute time, or "" if it
// can't be parsed.
func absTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02 15:04")
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

func shortHash(h string) string {
	h = strings.TrimPrefix(h, "sha256:")
	if len(h) > 16 {
		return h[:16] + "…"
	}
	return h
}

func compLabel(c string) string {
	switch strings.ToLower(c) {
	case "zstd", "zst":
		return "zst"
	case "gzip", "gz":
		return "gz"
	default:
		return c
	}
}

// relTime renders an RFC3339 timestamp as a short relative-or-absolute string.
func relTime(ts string) string {
	if ts == "" {
		return "—"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		if len(ts) > 10 {
			return ts[:10]
		}
		return ts
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

func truncate(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	var b strings.Builder
	for _, r := range s {
		if lipgloss.Width(b.String())+1 >= width {
			break
		}
		b.WriteRune(r)
	}
	return b.String() + "…"
}
