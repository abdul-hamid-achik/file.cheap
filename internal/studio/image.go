package studio

import (
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	// Register the decoders we support so image.Decode recognizes them. These are
	// all pure-Go (CGO-free) stdlib codecs.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// imageExts are the raster formats the preview pane can render inline.
var imageExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
}

// isImagePath reports whether rel names a renderable raster image by extension.
func isImagePath(rel string) bool {
	return imageExts[strings.ToLower(filepath.Ext(rel))]
}

// imageRefFromResult extracts a renderable image path from a search-result file
// locator. A direct hit ("frames/f.png") is returned as-is; a vidtrace per-frame
// unit label ("frames/f.png @ 12s") has its timestamp suffix stripped so the
// matching frame renders inline. Returns ("", false) for non-image locators.
func imageRefFromResult(locator string) (string, bool) {
	if isImagePath(locator) {
		return locator, true
	}
	if i := strings.LastIndex(locator, " @ "); i > 0 {
		if head := locator[:i]; isImagePath(head) {
			return head, true
		}
	}
	return "", false
}

// maxImagePixels caps the source image area we'll decode for a preview so a
// pathologically large image can't exhaust memory. ~24 MP covers 4K frames.
const maxImagePixels = 24 * 1000 * 1000

// imageBlend is the background color transparent pixels are composited over. It
// matches the studio's dark panel interior so cut-outs read as the surrounding UI.
var imageBlend = rgb{0x1e, 0x1e, 0x1e}

type rgb struct{ r, g, b uint8 }

// decodeImageFile decodes a stashed image file for preview rendering, rejecting
// images whose pixel area exceeds maxImagePixels before allocating the buffer.
func decodeImageFile(path string) (img image.Image, format string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = f.Close() }()

	cfg, format, err := image.DecodeConfig(f)
	if err != nil {
		return nil, "", err
	}
	if cfg.Width*cfg.Height > maxImagePixels {
		return nil, format, fmt.Errorf("image too large to preview (%d×%d)", cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, format, err
	}
	img, format, err = image.Decode(f)
	if err != nil {
		return nil, format, err
	}
	return img, format, nil
}

// renderImageBlocks renders img into at most cols×rows terminal cells using the
// upper-half-block technique: each cell stacks two vertical pixels (foreground =
// top, background = bottom), so one text row covers two image rows. Aspect ratio
// is preserved treating source pixels as square — a cell is roughly 1:2, so the
// two pixels it holds come out ~square. The result is horizontally centered and
// never upscaled. It emits raw 24-bit ANSI, which Ghostty and every truecolor
// terminal render; lipgloss measures it correctly (each ▀ is one cell wide).
func renderImageBlocks(img image.Image, cols, rows int) string {
	if img == nil || cols < 1 || rows < 1 {
		return ""
	}
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW < 1 || srcH < 1 {
		return ""
	}

	// Target pixel grid: cols wide, rows*2 tall. Fit within it, preserving the
	// source aspect ratio, and never enlarge a small image.
	maxPxW, maxPxH := cols, rows*2
	scale := math.Min(float64(maxPxW)/float64(srcW), float64(maxPxH)/float64(srcH))
	if scale > 1 {
		scale = 1
	}
	dstW := clamp(int(math.Round(float64(srcW)*scale)), 1, maxPxW)
	dstH := clamp(int(math.Round(float64(srcH)*scale)), 1, maxPxH)
	// Half-blocks need an even number of pixel rows (top+bottom per cell).
	if dstH%2 == 1 {
		if dstH < maxPxH {
			dstH++
		} else {
			dstH--
		}
	}
	if dstH < 2 {
		dstH = 2
	}

	grid := sampleImage(img, dstW, dstH)

	// Horizontal centering: pad each row to roughly center the image in the pane.
	pad := strings.Repeat(" ", (cols-dstW)/2)

	var sb strings.Builder
	for ty := 0; ty+1 < dstH; ty += 2 {
		top, bot := grid[ty], grid[ty+1]
		sb.WriteString(pad)
		for tx := 0; tx < dstW; tx++ {
			writeHalfBlock(&sb, top[tx], bot[tx])
		}
		sb.WriteString("\x1b[0m")
		if ty+3 < dstH {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// writeHalfBlock appends one ▀ cell: foreground paints the top pixel, background
// the bottom pixel.
func writeHalfBlock(sb *strings.Builder, top, bot rgb) {
	sb.WriteString("\x1b[38;2;")
	writeByteTriple(sb, top)
	sb.WriteString(";48;2;")
	writeByteTriple(sb, bot)
	sb.WriteString("m▀")
}

func writeByteTriple(sb *strings.Builder, c rgb) {
	sb.WriteString(strconv.Itoa(int(c.r)))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(int(c.g)))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(int(c.b)))
}

// sampleImage downscales img to dstW×dstH using a box (area-average) filter,
// compositing alpha over imageBlend. Averaging is done on the alpha-premultiplied
// values RGBA() returns, which is the correct way to downscale with transparency.
func sampleImage(img image.Image, dstW, dstH int) [][]rgb {
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	out := make([][]rgb, dstH)
	for ty := 0; ty < dstH; ty++ {
		row := make([]rgb, dstW)
		y0 := b.Min.Y + ty*srcH/dstH
		y1 := b.Min.Y + (ty+1)*srcH/dstH
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for tx := 0; tx < dstW; tx++ {
			x0 := b.Min.X + tx*srcW/dstW
			x1 := b.Min.X + (tx+1)*srcW/dstW
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var rs, gs, bs, as, count uint64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					cr, cg, cb, ca := img.At(x, y).RGBA() // 16-bit, alpha-premultiplied
					rs += uint64(cr)
					gs += uint64(cg)
					bs += uint64(cb)
					as += uint64(ca)
					count++
				}
			}
			if count == 0 {
				count = 1
			}
			row[tx] = compositeOverBlend(
				float64(rs/count), float64(gs/count), float64(bs/count), float64(as/count))
		}
		out[ty] = row
	}
	return out
}

// compositeOverBlend takes averaged 16-bit premultiplied components and composites
// them over imageBlend, returning an 8-bit color. result = premult + bg*(1-alpha).
func compositeOverBlend(r16, g16, b16, a16 float64) rgb {
	alpha := a16 / 65535.0
	inv := 1 - alpha
	const to16 = 257.0 // 8-bit -> 16-bit scale
	return rgb{
		r: to8((r16 + float64(imageBlend.r)*to16*inv) / to16),
		g: to8((g16 + float64(imageBlend.g)*to16*inv) / to16),
		b: to8((b16 + float64(imageBlend.b)*to16*inv) / to16),
	}
}

func to8(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// imageCaption renders a muted one-line summary shown under an image preview,
// e.g. "PNG · 1280×720 · 134.1 KiB".
func imageCaption(img image.Image, format string, size int64) string {
	b := img.Bounds()
	parts := fmt.Sprintf("%s · %d×%d", strings.ToUpper(format), b.Dx(), b.Dy())
	if size > 0 {
		parts += " · " + formatSize(size)
	}
	return mutedStyle.Render(parts)
}
