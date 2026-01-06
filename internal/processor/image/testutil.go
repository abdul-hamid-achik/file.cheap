package image

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
)

// createTestImage creates a test image with a gradient pattern.
// The gradient makes it easy to verify transformations visually.
func createTestImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Create a gradient pattern
			r := uint8(255 * x / width)
			g := uint8(255 * y / height)
			b := uint8(128)
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}

	return img
}

// createSolidColorImage creates a test image with a solid color.
func createSolidColorImage(width, height int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}

	return img
}

// encodeTestJPEG encodes an image as JPEG and returns a reader.
func encodeTestJPEG(img image.Image, quality int) io.Reader {
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	return bytes.NewReader(buf.Bytes())
}

// encodeTestPNG encodes an image as PNG and returns a reader.
func encodeTestPNG(img image.Image) io.Reader {
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return bytes.NewReader(buf.Bytes())
}

// createTestJPEG creates a JPEG image of the specified size.
func createTestJPEG(width, height int) io.Reader {
	img := createTestImage(width, height)
	return encodeTestJPEG(img, 85)
}

// createTestPNG creates a PNG image of the specified size.
func createTestPNG(width, height int) io.Reader {
	img := createTestImage(width, height)
	return encodeTestPNG(img)
}

// createLandscapeImage creates a wide image (width > height).
func createLandscapeImage() io.Reader {
	return createTestJPEG(800, 400)
}

// createPortraitImage creates a tall image (height > width).
func createPortraitImage() io.Reader {
	return createTestJPEG(400, 800)
}

// createSquareImage creates a square image.
func createSquareImage() io.Reader {
	return createTestJPEG(600, 600)
}

// createSmallImage creates a small test image.
func createSmallImage() io.Reader {
	return createTestJPEG(100, 100)
}

// createLargeImage creates a larger test image.
func createLargeImage() io.Reader {
	return createTestJPEG(2000, 1500)
}

// createInvalidImage returns data that is not a valid image.
func createInvalidImage() io.Reader {
	return bytes.NewReader([]byte("this is not an image"))
}

// createEmptyReader returns an empty reader.
func createEmptyReader() io.Reader {
	return bytes.NewReader([]byte{})
}

// createCorruptedJPEG returns a truncated JPEG (valid header, incomplete data).
func createCorruptedJPEG() io.Reader {
	// JPEG magic bytes followed by truncated data
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	return bytes.NewReader(data)
}

// getImageDimensions decodes an image and returns its dimensions.
func getImageDimensions(r io.Reader) (width, height int, err error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return 0, 0, err
	}
	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy(), nil
}

// readerToBytes reads all bytes from a reader.
func readerToBytes(r io.Reader) []byte {
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.Bytes()
}
