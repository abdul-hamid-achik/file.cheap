package studio

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/analyze"
	"github.com/abdul-hamid-achik/file.cheap/internal/manifest"
	"github.com/abdul-hamid-achik/file.cheap/internal/stash"
)

// solidImage builds a w×h image filled with c.
func solidImage(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestIsImagePath(t *testing.T) {
	for _, p := range []string{"a.png", "frames/f.PNG", "x.jpg", "x.jpeg", "x.gif"} {
		if !isImagePath(p) {
			t.Errorf("isImagePath(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"a.txt", "notes.md", "x.svg", "x.webp", "noext"} {
		if isImagePath(p) {
			t.Errorf("isImagePath(%q) = true, want false", p)
		}
	}
}

func TestDecodeImageFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "red.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, solidImage(8, 6, color.RGBA{255, 0, 0, 255})); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	img, format, err := decodeImageFile(p)
	if err != nil {
		t.Fatalf("decodeImageFile: %v", err)
	}
	if format != "png" {
		t.Errorf("format = %q, want png", format)
	}
	if b := img.Bounds(); b.Dx() != 8 || b.Dy() != 6 {
		t.Errorf("bounds = %dx%d, want 8x6", b.Dx(), b.Dy())
	}

	// A non-image file must error so the caller can fall back to the text reader.
	txt := filepath.Join(dir, "note.png") // .png ext but not a PNG
	if err := os.WriteFile(txt, []byte("not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeImageFile(txt); err == nil {
		t.Error("decodeImageFile(non-image) = nil error, want decode error")
	}
}

func TestRenderImageBlocks(t *testing.T) {
	red := solidImage(4, 4, color.RGBA{255, 0, 0, 255})
	out := renderImageBlocks(red, 38, 20)
	if out == "" {
		t.Fatal("renderImageBlocks returned empty")
	}
	if !strings.Contains(out, "▀") {
		t.Error("output missing the half-block glyph ▀")
	}
	// A solid red image must paint a pure-red foreground (top pixel).
	if !strings.Contains(out, "38;2;255;0;0") {
		t.Errorf("output missing red foreground escape, got:\n%q", out)
	}
	// No upscaling: a 4-px-wide source must not produce rows wider than 4 cells.
	for i, ln := range strings.Split(clean(out), "\n") {
		if cells := len([]rune(strings.TrimRight(ln, " "))); cells > 4+19 { // +centering pad
			t.Errorf("line %d is %d cells, suspiciously wide", i, cells)
		}
	}

	// Degenerate sizes must not panic and return empty.
	if renderImageBlocks(red, 0, 10) != "" || renderImageBlocks(nil, 10, 10) != "" {
		t.Error("degenerate inputs should render empty")
	}
}

func TestSampleImageAverages(t *testing.T) {
	// Left half red, right half blue; averaging one cell over the pair should give
	// a roughly equal red/blue mix (premultiplied-alpha averaging).
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	img.Set(1, 0, color.RGBA{0, 0, 255, 255})
	grid := sampleImage(img, 1, 1)
	got := grid[0][0]
	if got.r < 100 || got.r > 155 || got.b < 100 || got.b > 155 || got.g > 10 {
		t.Errorf("averaged pixel = %+v, want ~{127,0,127}", got)
	}
}

func TestImageCaption(t *testing.T) {
	cap := clean(imageCaption(solidImage(1280, 720, color.RGBA{0, 0, 0, 255}), "png", 137318))
	for _, want := range []string{"PNG", "1280×720", "KiB"} {
		if !strings.Contains(cap, want) {
			t.Errorf("caption %q missing %q", cap, want)
		}
	}
}

// TestImagePreviewInDetail wires a decoded image into a detail-view model and
// verifies the preview pane renders block art (not the binary-file message) and
// stays within the terminal bounds.
func TestImagePreviewInDetail(t *testing.T) {
	man := &manifest.Manifest{
		ID: "img_1", Name: "shot", Tool: "vidtrace",
		CreatedAt: "2026-06-23T06:00:00Z", FileCount: 1, TotalSize: 1000,
		Files: []manifest.FileEntry{{Path: "frames/frame_0001.png", Size: 134000}},
	}
	m := Model{width: 120, height: 40, activeView: viewDetail, focus: focusPreview,
		searchMode: "auto", selected: &stash.Stash{Manifest: man}}
	m.resize()
	m.previewImg = solidImage(64, 36, color.RGBA{30, 200, 120, 255})
	m.previewImgCap = imageCaption(m.previewImg, "png", 134000)

	out := m.render() // image rasterizes at View time via imageArt
	if !strings.Contains(out, "▀") {
		t.Error("detail view with an image should render half-block art")
	}
	if strings.Contains(clean(out), "not previewable") {
		t.Error("image preview should not show the binary-file message")
	}
	for i, ln := range strings.Split(clean(out), "\n") {
		if cells := len([]rune(ln)); cells > 120 {
			t.Errorf("line %d is %d cells wide, want <= 120", i, cells)
			break
		}
	}
	if got := strings.Count(clean(out), "\n") + 1; got != 40 {
		t.Errorf("rendered %d lines, want 40", got)
	}
}

// manyFileStash builds a stash whose manifest lists n numbered frame files.
func manyFileStash(n int) *stash.Stash {
	files := make([]manifest.FileEntry, n)
	for i := range files {
		files[i] = manifest.FileEntry{Path: fileName(i), Size: int64(i) * 1000}
	}
	return &stash.Stash{Manifest: &manifest.Manifest{
		ID: "big", Name: "big", Tool: "vidtrace", CreatedAt: "2026-06-23T06:00:00Z",
		FileCount: n, TotalSize: 99999, Files: files,
	}}
}

func fileName(i int) string {
	// zero-padded so substring matches are unambiguous (frame_0080, not frame_08).
	d := []byte("0000")
	for k := 3; k >= 0 && i > 0; k-- {
		d[k] = byte('0' + i%10)
		i /= 10
	}
	return "frames/frame_" + string(d) + ".png"
}

// TestFileTreeCursorStaysVisible is the regression test for the scroll bug: as the
// file cursor moves, the selected file must remain visible in the rendered detail
// view. Before the fix the scroll window was sized to the whole terminal, so the
// cursor scrolled off into the clipped region and the list appeared frozen.
func TestFileTreeCursorStaysVisible(t *testing.T) {
	st := manyFileStash(89)
	for _, idx := range []int{0, 20, 44, 70, 88} {
		m := Model{width: 120, height: 40, activeView: viewDetail, focus: focusFiles,
			searchMode: "auto", selected: st, fileIdx: idx}
		out := clean(m.render())
		want := fileName(idx) // selected file's path
		if !strings.Contains(out, want) {
			t.Errorf("fileIdx=%d: selected file %q not visible in detail view", idx, want)
		}
		// The cursor marker must be present (the selected row is on screen).
		if !strings.Contains(out, "▸ "+want) {
			t.Errorf("fileIdx=%d: cursor marker not on the selected row", idx)
		}
	}
}

// TestRenderFileTreeWindow checks the window is sized to the panel, not the
// terminal: it never emits more rows than the body height allows.
func TestRenderFileTreeWindow(t *testing.T) {
	m := Model{width: 120, height: 40, activeView: viewDetail, focus: focusFiles,
		selected: manyFileStash(50), fileIdx: 40}
	const bodyRows = 8
	out := clean(m.renderFileTree(bodyRows, 56))
	lines := strings.Split(out, "\n")
	if len(lines) > bodyRows {
		t.Errorf("renderFileTree emitted %d rows, want <= %d", len(lines), bodyRows)
	}
	// The selected file must be within the window.
	if !strings.Contains(out, fileName(40)) {
		t.Errorf("selected file not in window:\n%s", out)
	}
	// No row may exceed the interior width (regression: fixed 51-col rows wrapped
	// in the half-width left panel).
	for i, ln := range lines {
		if w := len([]rune(ln)); w > 56 {
			t.Errorf("row %d is %d cells wide, want <= 56", i, w)
		}
	}
}

// TestRenderImageShapeFidelity draws a red disc on a green field, samples it to a
// cell grid, and verifies the disc lands in the center and the corners stay green
// — i.e. the downscale preserves spatial structure. Run with -v to eyeball the map.
func TestRenderImageShapeFidelity(t *testing.T) {
	const s = 120
	img := image.NewRGBA(image.Rect(0, 0, s, s))
	cx, cy, r := s/2, s/2, s/3
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, color.RGBA{220, 40, 40, 255}) // red disc
			} else {
				img.Set(x, y, color.RGBA{40, 160, 60, 255}) // green field
			}
		}
	}

	const cols, rows = 40, 20
	grid := sampleImage(img, cols, rows*2)
	isRed := func(c rgb) bool { return c.r > c.g && c.r > 100 }

	var b strings.Builder
	for y := 0; y < len(grid); y += 2 { // one char per cell row
		for x := 0; x < len(grid[y]); x++ {
			if isRed(grid[y][x]) {
				b.WriteByte('#')
			} else {
				b.WriteByte('.')
			}
		}
		b.WriteByte('\n')
	}
	t.Logf("disc render:\n%s", b.String())

	center := grid[len(grid)/2][len(grid[0])/2]
	if !isRed(center) {
		t.Errorf("center cell = %+v, want red (disc not centered)", center)
	}
	for _, corner := range []rgb{grid[0][0], grid[0][len(grid[0])-1], grid[len(grid)-1][0]} {
		if isRed(corner) {
			t.Errorf("corner cell = %+v, want green field", corner)
		}
	}
}

// TestLoadFilePreviewDecodesImage exercises the full load path: a stashed PNG
// must come back as a decoded image (not a text/binary preview), while a text
// file comes back as content with no image.
func TestLoadFilePreviewDecodesImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "content", "frames"), 0o755); err != nil {
		t.Fatal(err)
	}
	imgPath := filepath.Join(dir, "content", "frames", "frame_0001.png")
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, solidImage(16, 12, color.RGBA{10, 20, 30, 255})); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if err := os.WriteFile(filepath.Join(dir, "content", "readme.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	man := &manifest.Manifest{ID: "s1", Files: []manifest.FileEntry{
		{Path: "frames/frame_0001.png", Size: 200},
		{Path: "readme.txt", Size: 5},
	}}
	m := Model{selected: &stash.Stash{Dir: dir, Manifest: man}, fileIdx: 0}

	msg, ok := m.loadFilePreviewCmd()().(previewLoadedMsg)
	if !ok {
		t.Fatal("expected previewLoadedMsg")
	}
	if msg.err != nil {
		t.Fatalf("image preview err: %v", msg.err)
	}
	if msg.img == nil {
		t.Fatal("PNG file did not decode into an image preview")
	}
	if msg.format != "png" {
		t.Errorf("format = %q, want png", msg.format)
	}

	m.fileIdx = 1 // the text file
	txtMsg := m.loadFilePreviewCmd()().(previewLoadedMsg)
	if txtMsg.img != nil {
		t.Error("text file should not produce an image preview")
	}
	if !strings.Contains(txtMsg.content, "hello") {
		t.Errorf("text preview = %q, want it to contain file content", txtMsg.content)
	}
}

// TestLoadResultPreviewDecodesImage verifies a search hit on an image file is
// rendered inline (decoded image), mirroring the detail pane.
func TestLoadResultPreviewDecodesImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "content", "frames"), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, "content", "frames", "hit.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, solidImage(12, 10, color.RGBA{200, 100, 50, 255})); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	// Both a direct image hit and a vidtrace per-frame locator ("… @ 12s") must
	// resolve to the rendered frame.
	for _, locator := range []string{"frames/hit.png", "frames/hit.png @ 12s"} {
		m := Model{
			stashes:       []*stash.Stash{{Dir: dir, Manifest: &manifest.Manifest{ID: "s1"}}},
			searchResults: []analyze.SearchResult{{StashID: "s1", File: locator, Score: 1.0}},
			resultIdx:     0,
		}
		msg, ok := m.loadResultPreviewCmd()().(previewLoadedMsg)
		if !ok {
			t.Fatalf("%q: expected previewLoadedMsg", locator)
		}
		if msg.img == nil {
			t.Fatalf("%q: image search hit did not decode into an image preview", locator)
		}
		if msg.format != "png" {
			t.Errorf("%q: format = %q, want png", locator, msg.format)
		}
	}
}

func TestImageRefFromResult(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"frames/f.png":       {"frames/f.png", true},
		"frames/f.png @ 12s": {"frames/f.png", true},
		"a/b/shot.jpeg @ 0s": {"a/b/shot.jpeg", true},
		"notes.txt":          {"", false},
		"transcript:derived": {"", false},
		"ocr/all.txt @ 3s":   {"", false}, // suffix present but head isn't an image
	}
	for in, exp := range cases {
		got, ok := imageRefFromResult(in)
		if ok != exp.ok || got != exp.want {
			t.Errorf("imageRefFromResult(%q) = (%q, %v), want (%q, %v)", in, got, ok, exp.want, exp.ok)
		}
	}
}

func TestSetFileCursorClamps(t *testing.T) {
	m := &Model{selected: manyFileStash(10), fileIdx: 5}
	m.setFileCursor(100)
	if m.fileIdx != 9 {
		t.Errorf("setFileCursor(100) clamped to %d, want 9", m.fileIdx)
	}
	m.moveFileCursor(-100)
	if m.fileIdx != 0 {
		t.Errorf("moveFileCursor(-100) clamped to %d, want 0", m.fileIdx)
	}
}
