package studio

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/abdul-hamid-achik/file.cheap/internal/detect"
	"github.com/abdul-hamid-achik/file.cheap/internal/diff"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

// previewLimitBytes caps how much of a file we read into the preview pane.
const previewLimitBytes = 64 * 1024

// loadStashesCmd lists all stashes from disk.
func loadStashesCmd(stashDir string) tea.Cmd {
	return func() tea.Msg {
		mgr, err := stash.NewManager(stashDir)
		if err != nil {
			return stashesLoadedMsg{err: err}
		}
		stashes, err := mgr.List(context.Background(), "")
		if err != nil {
			return stashesLoadedMsg{err: err}
		}
		return stashesLoadedMsg{stashes: stashes}
	}
}

// searchCmd runs a BM25 keyword search across all indexed stashes.
func (m *Model) searchCmd() tea.Cmd {
	query := strings.TrimSpace(m.query.Value())
	if query == "" {
		m.statusMessage = "enter a query"
		return nil
	}
	m.searching = true
	m.statusMessage = "searching..."
	m.errMessage = ""
	analyzer := m.analyzer
	ctx := m.ctx
	mode := m.searchMode
	if mode == "auto" {
		mode = ""
	}
	return func() tea.Msg {
		results, err := analyzer.Search(ctx, query, 0, mode)
		return searchDoneMsg{query: query, results: results, err: err}
	}
}

// loadFilePreviewCmd reads the currently selected file in the detail view.
func (m *Model) loadFilePreviewCmd() tea.Cmd {
	st := m.selected
	files := m.selectedFiles()
	if st == nil || m.fileIdx < 0 || m.fileIdx >= len(files) {
		return nil
	}
	rel := files[m.fileIdx].Path
	size := files[m.fileIdx].Size
	stashID := ""
	if st.Manifest != nil {
		stashID = st.Manifest.ID
	}
	compressed := st.Manifest != nil && st.Manifest.Compression != ""
	dir := st.Dir
	m.previewSeq++
	seq := m.previewSeq
	return func() tea.Msg {
		// Render raster images inline; fall through to the text reader on any
		// decode error (corrupt/unsupported file) so the pane still shows something.
		if !compressed && isImagePath(rel) {
			if img, format, err := decodeImageFile(filepath.Join(dir, "content", rel)); err == nil {
				return previewLoadedMsg{seq: seq, stashID: stashID, title: rel, img: img, format: format, size: size}
			}
		}
		content, err := readPreview(dir, rel, compressed)
		return previewLoadedMsg{seq: seq, stashID: stashID, title: rel, content: content, err: err}
	}
}

// loadResultPreviewCmd renders the snippet/content for the selected search hit.
func (m *Model) loadResultPreviewCmd() tea.Cmd {
	if m.resultIdx < 0 || m.resultIdx >= len(m.searchResults) {
		return nil
	}
	res := m.searchResults[m.resultIdx]
	// Find the owning stash to read full file content when available; keep its
	// file list so image hits can show a size in the caption (as the detail pane does).
	var dir string
	var compressed bool
	var files []manifest.FileEntry
	for _, st := range m.stashes {
		if st.Manifest != nil && st.Manifest.ID == res.StashID {
			dir = st.Dir
			compressed = st.Manifest.Compression != ""
			files = st.Manifest.Files
			break
		}
	}
	rel := res.File
	snippet := res.Text
	m.previewSeq++
	seq := m.previewSeq
	return func() tea.Msg {
		// Render image hits inline, just like the detail pane does. This also
		// covers vidtrace per-frame hits whose locator is "frames/f.png @ 12s".
		if dir != "" && !compressed {
			if imgRel, ok := imageRefFromResult(rel); ok {
				if img, format, err := decodeImageFile(filepath.Join(dir, "content", imgRel)); err == nil {
					var size int64
					for _, fe := range files {
						if fe.Path == imgRel {
							size = fe.Size
							break
						}
					}
					return previewLoadedMsg{seq: seq, stashID: res.StashID, title: imgRel, img: img, format: format, size: size}
				}
			}
		}
		header := fmt.Sprintf("%s › %s  (score %.2f)\n\n", res.StashID, rel, res.Score)
		if dir != "" && rel != "" && !compressed {
			if content, err := readPreview(dir, rel, false); err == nil {
				return previewLoadedMsg{seq: seq, stashID: res.StashID, title: rel, content: header + content}
			}
		}
		if snippet == "" {
			snippet = "(no snippet available)"
		}
		return previewLoadedMsg{seq: seq, stashID: res.StashID, title: rel, content: header + snippet}
	}
}

// diffCmd compares the active stash against a target directory.
func (m *Model) diffCmd(path string) tea.Cmd {
	st := m.currentStash()
	if st == nil {
		return nil
	}
	dir := st.Dir
	m.working = true
	m.statusMessage = "diffing..."
	m.errMessage = ""
	return func() tea.Msg {
		res, err := diff.CompareStashToDir(dir, path)
		if err != nil {
			return diffDoneMsg{err: err}
		}
		return diffDoneMsg{content: formatDiff(res, path)}
	}
}

// formatDiff renders a diff result into a styled, scrollable block.
func formatDiff(r *diff.DiffResult, path string) string {
	var b strings.Builder
	b.WriteString(dimStyle.Render("vs "+path) + "\n\n")
	if len(r.OnlyInStash) > 0 {
		b.WriteString(titleStyle.Render("Only in stash") + "\n")
		for _, f := range r.OnlyInStash {
			b.WriteString(goodStyle.Render("  + "+f) + "\n")
		}
		b.WriteString("\n")
	}
	if len(r.OnlyInTarget) > 0 {
		b.WriteString(titleStyle.Render("Only in target") + "\n")
		for _, f := range r.OnlyInTarget {
			b.WriteString(errorStyle.Render("  - "+f) + "\n")
		}
		b.WriteString("\n")
	}
	if len(r.Changed) > 0 {
		b.WriteString(titleStyle.Render("Changed") + "\n")
		for _, f := range r.Changed {
			b.WriteString(warnStyle.Render("  ~ "+f.Path) + "\n")
		}
		b.WriteString("\n")
	}
	summary := fmt.Sprintf("Unchanged: %d", r.Unchanged)
	if len(r.OnlyInStash) == 0 && len(r.OnlyInTarget) == 0 && len(r.Changed) == 0 {
		summary += "   " + goodStyle.Render("(identical)")
	}
	b.WriteString(mutedStyle.Render(summary))
	return b.String()
}

// indexFiles reports whether the SQLite metadata index and veclite search index
// files exist in the stash directory.
func (m Model) indexFiles() (hasDB, hasVec bool) {
	if _, err := os.Stat(filepath.Join(m.stashDir, "fcheap.db")); err == nil {
		hasDB = true
	}
	if _, err := os.Stat(filepath.Join(m.stashDir, "fcheap.veclite")); err == nil {
		hasVec = true
	}
	return
}

// loadTimelineCmd reads and formats a vidtrace bundle's evidence timeline.
func (m *Model) loadTimelineCmd() tea.Cmd {
	st := m.selected
	if st == nil {
		return nil
	}
	compressed := st.Manifest != nil && st.Manifest.Compression != ""
	dir := st.Dir
	return func() tea.Msg {
		if compressed {
			return timelineLoadedMsg{err: fmt.Errorf("stash is compressed — restore it to view the timeline")}
		}
		entries := detect.ParseVidtraceTimeline(filepath.Join(dir, "content"))
		if len(entries) == 0 {
			return timelineLoadedMsg{err: fmt.Errorf("no timeline entries found")}
		}
		return timelineLoadedMsg{count: len(entries), content: formatTimeline(entries)}
	}
}

// formatTimeline renders timeline entries into a styled, scrollable block.
func formatTimeline(entries []detect.TimelineEntry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(timeStyle.Render(fmt.Sprintf("%5.0fs", e.TimeSeconds)) + "  " + frameStyle.Render(e.Frame) + "\n")
		if e.OCR != "" {
			b.WriteString(labelStyle.Render("  ocr ") + inkStyle.Render(e.OCR) + "\n")
		}
		if e.Transcript != "" {
			b.WriteString(labelStyle.Render("  say ") + dimStyle.Render(e.Transcript) + "\n")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// readPreview returns a display-ready slice of a stashed file's content.
func readPreview(stashDir, rel string, compressed bool) (string, error) {
	if compressed {
		return "(compressed — restore to view)", nil
	}
	path := filepath.Join(stashDir, "content", rel)
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", rel, err)
	}
	defer func() { _ = f.Close() }()

	// ReadFull (vs a single Read, which may return a short buffer) so a file
	// larger than the limit reliably fills the buffer and is marked truncated.
	buf := make([]byte, previewLimitBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", fmt.Errorf("read %s: %w", rel, err)
	}
	data := buf[:n]
	if !looksTextual(data) {
		return "(binary file — not previewable)", nil
	}
	content := string(data)
	// The buffer filled exactly: probe one more byte to tell "exactly the limit"
	// from "there's more" so we only show the marker when content is actually cut.
	if err == nil {
		var extra [1]byte
		if m, _ := f.Read(extra[:]); m > 0 {
			content += "\n\n… (truncated)"
		}
	}
	return content, nil
}

// looksTextual is a cheap heuristic: reject content with NUL bytes.
func looksTextual(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

// restoreCmd restores the active stash to a temp directory and reports verify info.
func (m *Model) restoreCmd() tea.Cmd {
	st := m.currentStash()
	if st == nil || st.Manifest == nil {
		return nil
	}
	id := st.Manifest.ID
	stashDir := m.stashDir
	ctx := m.ctx
	m.working = true
	m.statusMessage = "restoring..."
	m.errMessage = ""
	return func() tea.Msg {
		mgr, err := stash.NewManager(stashDir)
		if err != nil {
			return actionDoneMsg{kind: "restore", err: err}
		}
		target := filepath.Join(os.TempDir(), "fcheap-restore-"+id)
		res, err := mgr.Restore(ctx, id, target)
		if err != nil {
			return actionDoneMsg{kind: "restore", err: err}
		}
		verify := "verified"
		if !res.Verified {
			verify = fmt.Sprintf("%d mismatch(es)!", len(res.Mismatches))
		}
		msg := fmt.Sprintf("restored %d file(s) to %s [%s]", res.FileCount, res.Target, verify)
		return actionDoneMsg{kind: "restore", message: msg}
	}
}

// dropCmd deletes the active stash from disk.
func (m *Model) dropCmd() tea.Cmd {
	st := m.currentStash()
	if st == nil || st.Manifest == nil {
		return nil
	}
	id := st.Manifest.ID
	stashDir := m.stashDir
	ctx := m.ctx
	m.working = true
	m.statusMessage = "dropping..."
	m.errMessage = ""
	return func() tea.Msg {
		mgr, err := stash.NewManager(stashDir)
		if err != nil {
			return actionDoneMsg{kind: "drop", err: err}
		}
		if err := mgr.Drop(ctx, id); err != nil {
			return actionDoneMsg{kind: "drop", err: err}
		}
		return actionDoneMsg{kind: "drop", message: "dropped " + id}
	}
}

// compressCmd archives the active stash with zstd and reports the savings.
func (m *Model) compressCmd() tea.Cmd {
	st := m.currentStash()
	if st == nil || st.Manifest == nil {
		return nil
	}
	if st.Manifest.Compression != "" {
		m.statusMessage = "already compressed"
		return nil
	}
	id := st.Manifest.ID
	stashDir := m.stashDir
	ctx := m.ctx
	m.working = true
	m.statusMessage = "compressing..."
	m.errMessage = ""
	return func() tea.Msg {
		mgr, err := stash.NewManager(stashDir)
		if err != nil {
			return actionDoneMsg{kind: "compress", err: err}
		}
		res, err := mgr.Compress(ctx, id, "zstd")
		if err != nil {
			return actionDoneMsg{kind: "compress", err: err}
		}
		saved := 0.0
		if res.OriginalSize > 0 {
			saved = (1 - float64(res.CompressedSize)/float64(res.OriginalSize)) * 100
		}
		msg := fmt.Sprintf("compressed %s: %s → %s (saved %.1f%%)",
			id, formatSize(res.OriginalSize), formatSize(res.CompressedSize), saved)
		return actionDoneMsg{kind: "compress", message: msg}
	}
}

// indexCmd indexes the active stash for keyword search, streaming per-file
// progress to drive an animated progress bar.
func (m *Model) indexCmd() tea.Cmd {
	// One index at a time: a second run would share m.indexing/progress state with
	// the first, interleaving the bar and clearing "indexing" while work continues.
	if m.indexing {
		m.statusMessage = "indexing already in progress"
		return nil
	}
	st := m.currentStash()
	if st == nil || st.Manifest == nil {
		return nil
	}
	dir := st.Dir
	id := st.Manifest.ID
	analyzer := m.analyzer
	ctx := m.ctx
	m.indexing = true
	m.indexDone, m.indexTotal = 0, 0
	m.statusMessage = "indexing " + id + "..."
	m.errMessage = ""

	ch := make(chan indexProg, 64)
	task := func() tea.Msg {
		defer close(ch)
		res, err := analyzer.IndexStashWithProgress(ctx, dir, func(done, total int) {
			select {
			case ch <- indexProg{done: done, total: total}:
			default: // drop updates if the UI is behind; the next one supersedes it
			}
		})
		if err != nil {
			return indexDoneMsg{err: err}
		}
		return indexDoneMsg{message: fmt.Sprintf("indexed %s: %d file(s) [%s]", res.StashID, res.FilesIndex, res.BundleType)}
	}
	return tea.Batch(waitForIndexProgress(ch), task)
}

// waitForIndexProgress blocks for the next progress update and re-arms itself.
func waitForIndexProgress(ch chan indexProg) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return indexProgressClosedMsg{}
		}
		return indexProgressMsg{done: p.done, total: p.total, ch: ch}
	}
}
