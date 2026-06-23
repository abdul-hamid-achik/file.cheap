package studio

import (
	"fmt"
	"image"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// hasFrames reports whether the selected stash holds a playable image sequence
// (≥2 raster files), which enables the video player.
func (m Model) hasFrames() bool {
	n := 0
	for _, f := range m.selectedFiles() {
		if isImagePath(f.Path) {
			if n++; n >= 2 {
				return true
			}
		}
	}
	return false
}

// startPlayback begins animating the stash's image frames in the preview pane,
// starting at the currently-selected frame (or the first). Needs ≥2 images.
func (m *Model) startPlayback() tea.Cmd {
	files := m.selectedFiles()
	frames := make([]int, 0, len(files))
	for i, f := range files {
		if isImagePath(f.Path) {
			frames = append(frames, i)
		}
	}
	if len(frames) < 2 {
		m.statusMessage = "no frame sequence to play"
		return nil
	}
	m.playFrames = frames
	// Resume from the currently-selected frame when it is part of the sequence.
	m.playPos = 0
	for i, fi := range frames {
		if fi == m.fileIdx {
			m.playPos = i
			break
		}
	}
	m.playFPS = m.frameRate()
	m.playing = true
	return m.playFrameCmd(m.playPos)
}

// stopPlayback halts playback. Any frame still decoding is dropped by the
// videoFrameMsg handler's !playing / stale-position guard.
func (m *Model) stopPlayback() {
	m.playing = false
}

// frameRate resolves the playback fps from the manifest's frame_rate, capped for
// terminal smoothness and per-frame decode cost.
func (m *Model) frameRate() int {
	fps := 10
	if m.selected != nil && m.selected.Manifest != nil {
		if v := strings.TrimSpace(m.selected.Manifest.Custom["frame_rate"]); v != "" {
			// frame_rate may be "30", "30.0", or "29.97" — take the integer part.
			if intPart := strings.SplitN(v, ".", 2)[0]; intPart != "" {
				if f, err := strconv.Atoi(intPart); err == nil && f > 0 {
					fps = f
				}
			}
		}
	}
	return clamp(fps, 1, 12)
}

// playFrameCmd schedules decoding of the frame at position pos after one frame
// interval, so frames advance at ~playFPS. Decoding happens off the UI goroutine.
func (m *Model) playFrameCmd(pos int) tea.Cmd {
	if !m.playing || pos < 0 || pos >= len(m.playFrames) {
		return nil
	}
	files := m.selectedFiles()
	fi := m.playFrames[pos]
	if fi < 0 || fi >= len(files) {
		return nil
	}
	rel := files[fi].Path
	size := files[fi].Size
	dir := ""
	if m.selected != nil {
		dir = m.selected.Dir
	}
	fps := m.playFPS
	if fps < 1 {
		fps = 1
	}
	delay := time.Second / time.Duration(fps)
	return func() tea.Msg {
		time.Sleep(delay)
		img, format, err := decodeImageFile(filepath.Join(dir, "content", rel))
		return videoFrameMsg{pos: pos, img: img, format: format, size: size, err: err}
	}
}

// playCaption is the caption shown under a frame during playback, including the
// frame counter so the user sees progress through the sequence.
func (m Model) playCaption(img image.Image, format string, size int64) string {
	b := img.Bounds()
	s := fmt.Sprintf("▶ %s · %d×%d · frame %d/%d", strings.ToUpper(format), b.Dx(), b.Dy(), m.playPos+1, len(m.playFrames))
	return mutedStyle.Render(s)
}
