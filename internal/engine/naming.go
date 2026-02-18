package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
)

// GenerateOutputPath produces an output filename based on the processor name
// and input path. For example, resize on "photo.jpg" yields "photo_resized.jpg".
func GenerateOutputPath(inputPath, processorName string, opts *processor.Options) string {
	dir := filepath.Dir(inputPath)
	ext := filepath.Ext(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), ext)

	var outName string

	switch processorName {
	case "resize":
		outName = base + "_resized" + ext
	case "thumbnail":
		outName = base + "_thumb" + ext
	case "webp":
		outName = base + ".webp"
	case "optimize":
		outName = base + "_optimized" + ext
	case "convert":
		format := "png"
		if opts != nil && opts.Format != "" {
			format = opts.Format
		}
		outName = base + "." + format
	case "watermark":
		outName = base + "_watermarked" + ext
	case "pixelart":
		format := "png"
		if opts != nil && opts.Format != "" {
			format = opts.Format
		}
		outName = base + "_pixelart." + format
	case "metadata":
		// Metadata extraction produces no output file.
		return ""
	case "pdf_thumbnail":
		outName = base + "_thumb.png"
	case "video_thumbnail":
		outName = base + "_thumb.jpg"
	case "video_transcode":
		outName = base + "_transcoded.mp4"
	default:
		outName = base + "_processed" + ext
	}

	return uniquePath(filepath.Join(dir, outName))
}

// fileExists reports whether path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// uniquePath returns a path that does not collide with an existing file.
// If "photo_resized.jpg" exists it tries "photo_resized_1.jpg", etc.
func uniquePath(path string) string {
	if !fileExists(path) {
		return path
	}

	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)

	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if !fileExists(candidate) {
			return candidate
		}
	}
}
