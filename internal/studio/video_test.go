package studio

import (
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

func TestHasFrames(t *testing.T) {
	if !(Model{selected: manyFileStash(2)}).hasFrames() {
		t.Error("a stash with ≥2 images should report hasFrames")
	}
	txt := &stash.Stash{Manifest: &manifest.Manifest{Files: []manifest.FileEntry{{Path: "a.txt"}, {Path: "b.md"}}}}
	if (Model{selected: txt}).hasFrames() {
		t.Error("a text-only stash should not report hasFrames")
	}
	if (Model{}).hasFrames() {
		t.Error("no selection should not report hasFrames")
	}
}

func TestFrameRate(t *testing.T) {
	mk := func(v string) *Model {
		man := &manifest.Manifest{}
		if v != "" {
			man.Custom = map[string]string{"frame_rate": v}
		}
		return &Model{selected: &stash.Stash{Manifest: man}}
	}
	cases := map[string]int{
		"30":    12, // capped
		"29.97": 12, // integer part, capped
		"5":     5,
		"0":     10, // invalid -> default
		"":      10, // missing -> default
		"abc":   10, // unparseable -> default
	}
	for in, want := range cases {
		if got := mk(in).frameRate(); got != want {
			t.Errorf("frameRate(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestStartPlaybackResumesAtCursor(t *testing.T) {
	m := &Model{selected: manyFileStash(5), fileIdx: 2, previewImgCache: &imgCache{}}
	m.selected.Manifest.Custom = map[string]string{"frame_rate": "30"}
	cmd := m.startPlayback()
	if cmd == nil {
		t.Fatal("startPlayback returned no command")
	}
	if !m.playing {
		t.Error("playing flag not set")
	}
	if len(m.playFrames) != 5 {
		t.Errorf("playFrames = %d, want 5", len(m.playFrames))
	}
	if m.playPos != 2 {
		t.Errorf("playPos = %d, want 2 (resume at the selected frame)", m.playPos)
	}
	if m.playFPS != 12 {
		t.Errorf("playFPS = %d, want 12 (capped)", m.playFPS)
	}
}

func TestStartPlaybackRefusedWithoutSequence(t *testing.T) {
	one := &stash.Stash{Manifest: &manifest.Manifest{Files: []manifest.FileEntry{{Path: "frames/only.png"}}}}
	m := &Model{selected: one, previewImgCache: &imgCache{}}
	if cmd := m.startPlayback(); cmd != nil || m.playing {
		t.Error("playback should be refused with fewer than 2 frames")
	}
}

func TestVideoFrameAdvancesAndSyncsCursor(t *testing.T) {
	m := Model{
		playing: true, activeView: viewDetail, selected: manyFileStash(3),
		playFrames: []int{0, 1, 2}, playPos: 1, playFPS: 5, previewImgCache: &imgCache{},
	}
	img := solidImage(4, 4, testColor)
	res, cmd := m.Update(videoFrameMsg{pos: 1, img: img, format: "png"})
	rm := res.(Model)
	if rm.previewImg == nil {
		t.Error("frame image was not shown")
	}
	if rm.fileIdx != 1 {
		t.Errorf("fileIdx = %d, want 1 (cursor follows playback)", rm.fileIdx)
	}
	if rm.playPos != 2 {
		t.Errorf("playPos = %d, want 2 (advanced)", rm.playPos)
	}
	if cmd == nil {
		t.Error("expected a follow-up frame command")
	}
}

func TestVideoFrameDroppedWhenStopped(t *testing.T) {
	m := Model{playing: false, activeView: viewDetail, playFrames: []int{0, 1}, playPos: 0,
		selected: manyFileStash(2), previewImgCache: &imgCache{}}
	_, cmd := m.Update(videoFrameMsg{pos: 0, img: solidImage(4, 4, testColor), format: "png"})
	if cmd != nil {
		t.Error("a frame arriving after stop must not re-arm the loop")
	}

	// A stale-position frame (user resumed elsewhere) is also dropped.
	m.playing = true
	m.playPos = 1
	_, cmd = m.Update(videoFrameMsg{pos: 0, img: solidImage(4, 4, testColor), format: "png"})
	if cmd != nil {
		t.Error("a stale-position frame must not re-arm the loop")
	}
}

func TestNavigationStopsPlayback(t *testing.T) {
	m := &Model{playing: true, activeView: viewDetail, selected: manyFileStash(4),
		fileIdx: 0, previewImgCache: &imgCache{}}
	m.handleDetailKey("j") // any non-toggle key hands back control
	if m.playing {
		t.Error("navigating the files pane should stop playback")
	}
}

func TestPlayToggle(t *testing.T) {
	m := &Model{activeView: viewDetail, selected: manyFileStash(3), previewImgCache: &imgCache{}}
	if cmd, _ := m.handleDetailKey("p"); cmd == nil || !m.playing {
		t.Error("p should start playback")
	}
	if _, _ = m.handleDetailKey("p"); m.playing {
		t.Error("p again should stop playback")
	}
}
