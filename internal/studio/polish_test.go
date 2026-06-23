package studio

import (
	"errors"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
)

// TestPlaybackRefusedOnCompressed: a compressed stash has no on-disk frames, so
// the player must refuse to start (no silent decode-fails-forever loop) and not
// advertise the key.
func TestPlaybackRefusedOnCompressed(t *testing.T) {
	st := manyFileStash(5)
	st.Manifest.Compression = "zstd"
	m := &Model{selected: st, previewImgCache: &imgCache{}}
	if m.hasFrames() {
		t.Error("compressed stash should not report hasFrames")
	}
	if cmd := m.startPlayback(); cmd != nil || m.playing {
		t.Error("startPlayback must refuse a compressed stash")
	}
}

// TestStartPlaybackBumpsPreviewSeq: starting playback invalidates any in-flight
// file/result preview load so it can't clobber the first frame.
func TestStartPlaybackBumpsPreviewSeq(t *testing.T) {
	m := &Model{selected: manyFileStash(5), previewSeq: 7, previewImgCache: &imgCache{}}
	m.startPlayback()
	if m.previewSeq == 7 {
		t.Error("startPlayback should bump previewSeq")
	}
}

// TestVideoFrameStopsOnDecodeError: a frame that fails to decode mid-play stops
// playback instead of re-arming a doomed loop.
func TestVideoFrameStopsOnDecodeError(t *testing.T) {
	m := Model{
		playing: true, activeView: viewDetail, selected: manyFileStash(3),
		playFrames: []int{0, 1, 2}, playPos: 1, playFPS: 5, previewImgCache: &imgCache{},
	}
	res, cmd := m.Update(videoFrameMsg{pos: 1, err: errors.New("missing")})
	if res.(Model).playing {
		t.Error("a decode error during playback should stop playback")
	}
	if cmd != nil {
		t.Error("must not re-arm the frame loop after a decode error")
	}
}

// TestStopPlaybackPauseGlyph: pausing swaps the ▶ caption glyph to ⏸ so the state
// is visible, and invalidates the art cache so the new caption renders.
func TestStopPlaybackPauseGlyph(t *testing.T) {
	m := &Model{playing: true, previewImgCache: &imgCache{img: solidImage(2, 2, testColor)}}
	m.previewImgCap = m.playCaption(solidImage(4, 4, testColor), "png", 0) // begins with ▶
	m.stopPlayback()
	if strings.Contains(m.previewImgCap, "▶") || !strings.Contains(m.previewImgCap, "⏸") {
		t.Errorf("paused caption should swap ▶→⏸, got %q", clean(m.previewImgCap))
	}
	if m.previewImgCache.img != nil {
		t.Error("stopPlayback should invalidate the art cache so the new caption shows")
	}
}

// TestPreviewLoadGatedToDetailSearch: a late preview load that lands after a
// switch to a non-preview view (diff/timeline/list) must not be applied.
func TestPreviewLoadGatedToDetailSearch(t *testing.T) {
	img := solidImage(4, 4, testColor)
	gated := Model{activeView: viewTimeline, previewSeq: 1, previewImgCache: &imgCache{}}
	if res, _ := gated.Update(previewLoadedMsg{seq: 1, img: img}); res.(Model).previewImg != nil {
		t.Error("preview load applied in timeline view (should be gated out)")
	}
	ok := Model{activeView: viewDetail, previewSeq: 1, previewImgCache: &imgCache{}}
	if res, _ := ok.Update(previewLoadedMsg{seq: 1, img: img}); res.(Model).previewImg == nil {
		t.Error("preview load not applied in detail view")
	}
}

// TestSearchResultsJumpKeys: g/G jump to first/last result (parity with the list
// and files panes), not just scroll a focused preview.
func TestSearchResultsJumpKeys(t *testing.T) {
	m := &Model{activeView: viewSearch, focus: focusResults, resultIdx: 1,
		searchResults: []analyze.SearchResult{{StashID: "a"}, {StashID: "b"}, {StashID: "c"}}}
	m.handleSearchKey("G")
	if m.resultIdx != 2 {
		t.Errorf("G: resultIdx = %d, want 2", m.resultIdx)
	}
	m.handleSearchKey("g")
	if m.resultIdx != 0 {
		t.Errorf("g: resultIdx = %d, want 0", m.resultIdx)
	}
}

// TestNarrowTerminalGuard: below the panel minimum width, render shows a clipped
// notice rather than overflowing bordered boxes.
func TestNarrowTerminalGuard(t *testing.T) {
	m := Model{width: 16, height: 10, activeView: viewList, searchMode: "auto"}
	out := clean(m.render())
	if !strings.Contains(out, "narrow") {
		t.Errorf("expected a too-narrow notice, got: %q", out)
	}
	for i, ln := range strings.Split(out, "\n") {
		if c := len([]rune(ln)); c > 16 {
			t.Errorf("line %d is %d cells, want <= 16", i, c)
		}
	}
}

// TestStackedImagePreviewFitsNarrow: the stacked image preview must not overflow
// the terminal width or grow the body past the terminal height at narrow widths
// (regression for the viewport-interior width-floor bug).
func TestStackedImagePreviewFitsNarrow(t *testing.T) {
	st := manyFileStash(5)
	for _, w := range []int{20, 24, 30, 50, 80} {
		m := Model{width: w, height: 30, activeView: viewDetail, focus: focusPreview,
			searchMode: "auto", selected: st,
			previewImg: solidImage(40, 30, testColor), previewImgCache: &imgCache{}}
		lines := strings.Split(clean(m.render()), "\n")
		for i, ln := range lines {
			if c := len([]rune(ln)); c > w {
				t.Errorf("w=%d: line %d is %d cells, want <= %d", w, i, c, w)
				break
			}
		}
		if len(lines) != 30 {
			t.Errorf("w=%d: rendered %d lines, want 30 (body grew past the terminal)", w, len(lines))
		}
	}
}
