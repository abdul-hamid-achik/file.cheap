package studio

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

	header := m.renderHeader()
	var body string
	switch m.activeView {
	case viewDetail:
		body = m.renderDetail()
	case viewSearch:
		body = m.renderSearch()
	case viewTimeline:
		body = m.renderTimeline()
	case viewDiff:
		body = m.renderDiff()
	case viewStatus:
		body = m.renderStatus()
	case viewHelp:
		body = m.renderHelp()
	default:
		body = m.renderList()
	}
	footer := m.renderFooter()
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// --- header ---

func (m Model) renderHeader() string {
	left := brandStyle.Render("fcheap") + " " + titleStyle.Render("studio")
	right := mutedStyle.Render(fmt.Sprintf("%d stash(es)", len(m.stashes)))
	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad > 1 {
		return left + strings.Repeat(" ", pad) + right
	}
	return left
}

// --- list view ---

func (m Model) renderList() string {
	if m.loading {
		return panelStyle.Width(m.width - 2).Render(m.spinner.View() + " " + mutedStyle.Render("loading stashes…"))
	}
	if len(m.stashes) == 0 {
		hint := lipgloss.JoinVertical(lipgloss.Left,
			mutedStyle.Render("No stashes yet."),
			"",
			dimStyle.Render("Create one with:"),
			keyStyle.Render("  fcheap save <path>"),
		)
		return panelStyle.Width(m.width - 2).Render(hint)
	}

	rows := m.renderStashRows(m.width - 4)
	return m.renderPanel("Stashes", rows, m.width-2, true)
}

func (m Model) renderStashRows(width int) string {
	var b strings.Builder
	maxRows := clamp(m.height-8, 5, len(m.stashes))
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	end := clamp(start+maxRows, 0, len(m.stashes))

	for i := start; i < end; i++ {
		row := m.renderStashRow(i, width)
		if i == m.cursor && m.activeView == viewList {
			b.WriteString(selectedRowStyle.Width(width).Render(row))
		} else {
			b.WriteString(row)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderStashRow(i, width int) string {
	st := m.stashes[i]
	man := st.Manifest
	marker := "  "
	if i == m.cursor {
		marker = "▸ "
	}
	name := man.ID
	if man.Name != "" {
		name = man.Name
	}

	cols := []string{
		fmt.Sprintf("%s%-22s", marker, truncate(name, 22)),
		fmt.Sprintf("%-10s", truncate(man.Tool, 10)),
		fmt.Sprintf("%4d f", man.FileCount),
		fmt.Sprintf("%9s", formatSize(man.TotalSize)),
		fmt.Sprintf("%-12s", relTime(man.CreatedAt)),
	}
	row := strings.Join(cols, "  ")
	if man.Compression != "" {
		row += "  " + zstChipStyle.Render(compLabel(man.Compression))
	}
	if man.Custom["secrets_found"] != "" {
		row += "  " + warnChipStyle.Render("⚠ secrets")
	}
	return truncate(row, width)
}

// --- detail view ---

func (m Model) renderDetail() string {
	if m.selected == nil || m.selected.Manifest == nil {
		return panelStyle.Width(m.width - 2).Render(mutedStyle.Render("no stash selected"))
	}
	man := m.selected.Manifest

	var info strings.Builder
	addLine(&info, "ID", man.ID)
	if man.Name != "" {
		addLine(&info, "Name", man.Name)
	}
	if man.SourcePath != "" {
		addLine(&info, "Source", man.SourcePath)
	}
	if man.Tool != "" {
		addLine(&info, "Tool", man.Tool)
	}
	addLine(&info, "Created", relTime(man.CreatedAt))
	if man.BundleType != "" && man.BundleType != "generic" {
		info.WriteString(dimStyle.Render(fmt.Sprintf("%-9s", "Bundle:")) + bundleChipStyle(man.BundleType).Render(man.BundleType) + "\n")
	}
	if v := man.VideoSummary(); v != "" {
		addLine(&info, "Video", v)
	}
	addLine(&info, "Files", fmt.Sprintf("%d", man.FileCount))
	addLine(&info, "Size", formatSize(man.TotalSize))
	addLine(&info, "Hash", shortHash(man.ContentHash))
	if man.Compression != "" {
		addLine(&info, "Stored", fmt.Sprintf("%s (%s)", compLabel(man.Compression), formatSize(man.CompressedSize)))
	} else {
		addLine(&info, "Stored", "uncompressed")
	}
	if c := man.Custom["secrets_found"]; c != "" {
		info.WriteString(dimStyle.Render(fmt.Sprintf("%-9s", "Secrets:")) + warnChipStyle.Render("⚠ "+c+" potential") + "\n")
	}
	if len(man.Tags) > 0 {
		chips := make([]string, 0, len(man.Tags))
		for _, t := range man.Tags {
			chips = append(chips, tagChipStyle.Render(t))
		}
		info.WriteString(dimStyle.Render("Tags:   ") + strings.Join(chips, " ") + "\n")
	}

	files := m.renderFileTree()
	filesTitle := fmt.Sprintf("Files (%d)", man.FileCount)

	if m.width >= 96 {
		leftW := clamp(m.width/2-2, 30, m.width-4)
		rightW := m.width - leftW - 4
		left := lipgloss.JoinVertical(lipgloss.Left,
			m.sizePanel("Provenance", info.String(), leftW, false),
			m.sizePanel(filesTitle, files, leftW, m.focus == focusFiles),
		)
		right := m.sizePanel(m.previewTitle(), m.renderPreview(), rightW, m.focus == focusPreview)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		m.sizePanel("Provenance", info.String(), m.width-2, false),
		m.sizePanel(filesTitle, files, m.width-2, m.focus == focusFiles),
		m.sizePanel(m.previewTitle(), m.renderPreview(), m.width-2, m.focus == focusPreview),
	)
}

func (m Model) renderFileTree() string {
	files := m.selectedFiles()
	if len(files) == 0 {
		return mutedStyle.Render("(no files)")
	}
	maxRows := clamp(m.height/2-4, 4, 18)
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
		line := fmt.Sprintf("%s%-40s %8s", marker, truncate(files[i].Path, 40), formatSize(files[i].Size))
		if i == m.fileIdx && m.focus == focusFiles {
			b.WriteString(selectedRowStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	if end < len(files) {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  … %d more", len(files)-end)))
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
	return m.preview.View()
}

// --- search view ---

func (m Model) renderSearch() string {
	queryPanel := m.sizePanel("Search · mode "+m.searchMode, m.query.View(), m.width-2, m.focus == focusQuery)

	results := m.renderSearchResults()
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
		left := m.sizePanel(resultsTitle, results, leftW, m.focus == focusResults)
		right := m.sizePanel("Preview", m.renderPreview(), rightW, m.focus == focusPreview)
		return lipgloss.JoinVertical(lipgloss.Left,
			queryPanel,
			lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		queryPanel,
		m.sizePanel(resultsTitle, results, m.width-2, m.focus == focusResults),
		m.sizePanel("Preview", m.renderPreview(), m.width-2, m.focus == focusPreview),
	)
}

func (m Model) renderSearchResults() string {
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
	maxRows := clamp(m.height-12, 5, len(m.searchResults))
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

func (m Model) renderTimeline() string {
	title := "Timeline"
	if m.selected != nil && m.selected.Manifest != nil && m.selected.Manifest.Name != "" {
		title = "Timeline · " + m.selected.Manifest.Name
	}
	return m.sizePanel(title, m.preview.View(), m.width-2, true)
}

// --- diff view ---

func (m Model) renderDiff() string {
	title := "Diff"
	if m.selected != nil && m.selected.Manifest != nil && m.selected.Manifest.Name != "" {
		title = "Diff · " + m.selected.Manifest.Name
	}
	return m.sizePanel(title, m.preview.View(), m.width-2, true)
}

// --- status view ---

func (m Model) renderStatus() string {
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
	return m.sizePanel("Status", strings.Join(lines, "\n"), m.width-2, true)
}

// presence renders a green "present" / dim "—" indicator.
func presence(ok bool) string {
	if ok {
		return goodStyle.Render("present")
	}
	return mutedStyle.Render("—")
}

// --- help view ---

func (m Model) renderHelp() string {
	sections := []string{
		titleStyle.Render("Navigation"),
		help("j / k  ↑ / ↓", "move cursor"),
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
		help("d", "drop (confirm y/n)"),
		help("g", "refresh stash list"),
		"",
		titleStyle.Render("Search"),
		help("/", "focus query input"),
		help("enter", "run search"),
		help("m", "cycle mode (auto/keyword/semantic/hybrid)"),
		help("tab", "query ↔ results ↔ preview"),
	}
	return m.sizePanel("Help", strings.Join(sections, "\n"), m.width-2, true)
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
		hints = append(hints,
			keyHint("r", "restore"), keyHint("c", "compress"),
			keyHint("a", "index"), keyHint("x", "diff"), keyHint("d", "drop"), keyHint("esc", "back"),
		)
		return strings.Join(hints, "  ")
	case viewTimeline, viewDiff:
		return strings.Join([]string{
			keyHint("j/k", "scroll"), keyHint("esc", "back"), keyHint("q", "quit"),
		}, "  ")
	case viewSearch:
		return strings.Join([]string{
			keyHint("/", "query"), keyHint("enter", "search"), keyHint("tab", "focus"),
			keyHint("m", "mode"), keyHint("esc", "back"),
		}, "  ")
	case viewStatus, viewHelp:
		return keyHint("esc", "back")
	default:
		return strings.Join([]string{
			keyHint("enter", "open"), keyHint("/", "search"), keyHint("r", "restore"),
			keyHint("c", "compress"), keyHint("a", "index"), keyHint("x", "diff"),
			keyHint("d", "drop"), keyHint("s", "status"), keyHint("?", "help"), keyHint("q", "quit"),
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

// --- formatting helpers ---

func addLine(b *strings.Builder, label, value string) {
	b.WriteString(dimStyle.Render(fmt.Sprintf("%-9s", label+":")) + inkStyle.Render(value) + "\n")
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
