package studio

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

// testColor is an arbitrary opaque color for building preview fixtures.
var testColor = color.RGBA{30, 200, 120, 255}

// nStashes builds n minimal stashes for list-navigation tests.
func nStashes(n int) []*stash.Stash {
	out := make([]*stash.Stash, n)
	for i := range out {
		out[i] = &stash.Stash{Manifest: &manifest.Manifest{
			ID: fmt.Sprintf("s%02d", i), Name: fmt.Sprintf("s%02d", i), Tool: "generic",
			CreatedAt: "2026-06-23T06:00:00Z",
		}}
	}
	return out
}

// TestPreviewSeqDropsStaleLoads verifies the async race guard: a previewLoadedMsg
// whose seq no longer matches the model's current request is ignored.
func TestPreviewSeqDropsStaleLoads(t *testing.T) {
	img := solidImage(4, 4, testColor)
	m := Model{previewSeq: 5, previewImgCache: &imgCache{}, activeView: viewDetail}

	stale, _ := m.Update(previewLoadedMsg{seq: 4, img: img})
	if stale.(Model).previewImg != nil {
		t.Error("a stale (superseded) preview load was applied")
	}

	fresh, _ := m.Update(previewLoadedMsg{seq: 5, img: img})
	if fresh.(Model).previewImg == nil {
		t.Error("the current preview load was dropped")
	}
}

func TestZeroResultSearchClearsPreviousPreview(t *testing.T) {
	m := NewModel(context.Background(), t.TempDir(), "", analyze.EmbedderSettings{})
	m.activeView = viewSearch
	m.preview.SetContent("stale result content")
	m.previewImg = solidImage(2, 2, testColor)

	updated, cmd := m.Update(searchDoneMsg{query: "nothing", results: []analyze.SearchResult{}})
	got := updated.(Model)
	if cmd != nil {
		t.Fatal("zero-result search started a preview load")
	}
	if got.previewImg != nil {
		t.Fatal("zero-result search retained the previous image")
	}
	if view := clean(got.preview.View()); strings.Contains(view, "stale result content") || !strings.Contains(view, "No matching") {
		t.Fatalf("zero-result preview = %q", view)
	}
}

func TestPreviewRequestClearsOldContentWhileLoading(t *testing.T) {
	m := NewModel(context.Background(), t.TempDir(), "", analyze.EmbedderSettings{})
	m.activeView = viewDetail
	m.selected = manyFileStash(1)
	m.preview.SetContent("old file content")
	cmd := m.loadFilePreviewCmd()
	if cmd == nil {
		t.Fatal("loadFilePreviewCmd returned nil")
	}
	view := clean(m.preview.View())
	if strings.Contains(view, "old file content") || !strings.Contains(view, "Loading preview") {
		t.Fatalf("preview while loading = %q", view)
	}
}

// TestDiffEscReinitsDetail is the regression test for the high-severity diff→detail
// bug: returning from a diff must reset the file cursor and clear the stale
// image/diff content so the detail view isn't shown with another mode's state.
func TestDiffEscReinitsDetail(t *testing.T) {
	st := manyFileStash(5)
	m := &Model{
		activeView: viewDiff, focus: focusPreview, selected: st, fileIdx: 15,
		previewImg: solidImage(4, 4, testColor), previewImgCache: &imgCache{},
	}
	_, handled := m.handleDiffKey("esc")
	if !handled {
		t.Fatal("esc not handled in diff view")
	}
	if m.activeView != viewDetail || m.focus != focusFiles {
		t.Errorf("after esc: view=%v focus=%v, want detail/files", m.activeView, m.focus)
	}
	if m.fileIdx != 0 {
		t.Errorf("fileIdx = %d, want 0 (reset to a valid index)", m.fileIdx)
	}
	if m.previewImg != nil {
		t.Error("stale diff/image preview was not cleared on return to detail")
	}
}

// TestDetailFooterDropIsFocusAware: "d drop" must not be advertised while the
// preview pane is focused (there d is the pager, not a destructive drop).
func TestDetailFooterDropIsFocusAware(t *testing.T) {
	man := &manifest.Manifest{ID: "x", Name: "x"}
	files := Model{activeView: viewDetail, focus: focusFiles, selected: &stash.Stash{Manifest: man}}
	if got := clean(files.contextHints()); !strings.Contains(got, "drop") {
		t.Errorf("files-focus footer should advertise drop, got: %s", got)
	}
	prev := Model{activeView: viewDetail, focus: focusPreview, selected: &stash.Stash{Manifest: man}}
	if got := clean(prev.contextHints()); strings.Contains(got, "drop") {
		t.Errorf("preview-focus footer must not advertise drop, got: %s", got)
	}
}

// TestSearchFooterModeIsFocusAware: "m mode" is inert while typing in the query,
// so it must only appear once focus leaves the query input.
func TestSearchFooterModeIsFocusAware(t *testing.T) {
	q := Model{activeView: viewSearch, focus: focusQuery}
	if strings.Contains(clean(q.contextHints()), "mode") {
		t.Error("query-focus search footer must not advertise the dead 'm mode' key")
	}
	r := Model{activeView: viewSearch, focus: focusResults}
	if !strings.Contains(clean(r.contextHints()), "mode") {
		t.Error("results-focus search footer should advertise 'm mode'")
	}
}

// TestCycleSearchFocusNoResults: tab must not strand focus on empty panes.
func TestCycleSearchFocusNoResults(t *testing.T) {
	m := &Model{activeView: viewSearch, focus: focusQuery}
	m.cycleSearchFocus()
	if m.focus != focusQuery {
		t.Errorf("with no results, focus = %v, want focusQuery", m.focus)
	}
	m.searchResults = []analyze.SearchResult{{StashID: "s"}}
	m.cycleSearchFocus()
	if m.focus != focusResults {
		t.Errorf("with results, focus = %v, want focusResults", m.focus)
	}
}

// TestIndexCmdGuard: a second index while one is running is refused.
func TestIndexCmdGuard(t *testing.T) {
	m := &Model{indexing: true}
	if cmd := m.indexCmd(); cmd != nil {
		t.Error("indexCmd should return nil while an index is already in flight")
	}
}

// TestListJumpKeys: g/G jump to first/last stash (consistent with detail).
func TestListJumpKeys(t *testing.T) {
	m := &Model{activeView: viewList, stashes: nStashes(6), cursor: 3}
	m.handleListKey("G")
	if m.cursor != 5 {
		t.Errorf("G: cursor = %d, want 5", m.cursor)
	}
	m.handleListKey("g")
	if m.cursor != 0 {
		t.Errorf("g: cursor = %d, want 0", m.cursor)
	}
}

// TestReadPreviewTruncation: files larger than the limit get the marker; small
// files do not (regression for the single-Read short-read miss).
func TestReadPreviewTruncation(t *testing.T) {
	dir := t.TempDir()
	cdir := filepath.Join(dir, "content")
	if err := os.MkdirAll(cdir, 0o755); err != nil {
		t.Fatal(err)
	}
	big := strings.Repeat("a", previewLimitBytes+5000)
	if err := os.WriteFile(filepath.Join(cdir, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cdir, "small.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := readPreview(dir, "big.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "truncated") {
		t.Error("large file preview should be marked truncated")
	}
	small, err := readPreview(dir, "small.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(small, "truncated") {
		t.Error("small file preview should not be marked truncated")
	}
}

// TestPanelTitleTruncation: an over-long panel title is truncated to the interior
// so it can't wrap and clip the panel's bottom border.
func TestPanelTitleTruncation(t *testing.T) {
	m := Model{width: 40, height: 20}
	long := strings.Repeat("very-long-title-", 10)
	out := m.renderPanelH(long, "body", 30, 10, false)
	for i, ln := range strings.Split(clean(out), "\n") {
		if w := len([]rune(ln)); w > 30 {
			t.Errorf("line %d is %d cells, want <= 30 (title should be truncated)", i, w)
		}
	}
	if got := strings.Count(clean(out), "\n") + 1; got != 10 {
		t.Errorf("panel rendered %d lines, want 10 (title wrap would add a line)", got)
	}
}

// TestDetailNoOverflowAcrossWidths: the detail view (with many long-named files)
// must not emit any line wider than the terminal, at the widths where the old
// fixed-51-col file rows used to wrap.
func TestDetailNoOverflowAcrossWidths(t *testing.T) {
	st := manyFileStash(40)
	for _, w := range []int{96, 100, 108, 113, 120, 140} {
		m := Model{width: w, height: 40, activeView: viewDetail, focus: focusFiles,
			searchMode: "auto", selected: st, fileIdx: 5, previewImgCache: &imgCache{}}
		for i, ln := range strings.Split(clean(m.render()), "\n") {
			if cells := len([]rune(ln)); cells > w {
				t.Errorf("w=%d: line %d is %d cells wide, want <= %d", w, i, cells, w)
				break
			}
		}
	}
}

// TestDetailLongProvenanceKeepsFilesPanelInBounds reproduces a large generated
// bundle: long IDs, source paths, and tag rows wrap in the left column. Those
// wrapped rows must be included in the height budget or the Files panel loses
// its scroll indicator and bottom border below the terminal edge.
func TestDetailLongProvenanceKeepsFilesPanelInBounds(t *testing.T) {
	st := manyFileStash(80)
	st.Manifest.ID = strings.Repeat("2026_07_27t21_46_16_148z_generated_bundle_", 3)
	st.Manifest.SourcePath = "/workspace/examples/generated/bundles/" + strings.Repeat("long_fixture_name_", 8)
	st.Manifest.Tags = []string{
		"large-generated-bundle", "regression", "layout-fixture", "long-metadata",
	}

	const height = 44
	m := Model{width: 200, height: height, activeView: viewDetail, focus: focusFiles,
		searchMode: "auto", selected: st, previewImgCache: &imgCache{}}
	out := clean(m.renderDetail(height))
	lines := strings.Split(out, "\n")
	if got := len(lines); got != height {
		t.Fatalf("detail rendered %d lines, want %d; a wrapped provenance row overflowed the body", got, height)
	}
	if !strings.Contains(out, "more") {
		t.Error("overflowing file list has no visible continuation indicator")
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, "╰") || !strings.Contains(last, "╯") {
		t.Errorf("last detail row does not contain the focused Files panel border: %q", last)
	}
}
