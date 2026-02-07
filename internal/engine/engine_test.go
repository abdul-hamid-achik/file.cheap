package engine

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestJPEG writes a tiny solid-red JPEG to path and returns the path.
func createTestJPEG(t *testing.T, dir, name string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}))

	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0644))
	return path
}

func TestNewEngine(t *testing.T) {
	e := New(nil)
	require.NotNil(t, e)
	require.NotNil(t, e.Registry)
	require.NotNil(t, e.Config)
	assert.Equal(t, processor.DefaultConfig().Quality, e.Config.Quality)
}

func TestNewEngineWithConfig(t *testing.T) {
	cfg := &processor.Config{
		MaxFileSize:  50 * 1024 * 1024,
		TempDir:      "/tmp/engine-test",
		Quality:      90,
		MaxDimension: 2048,
	}
	e := New(cfg)
	assert.Equal(t, 90, e.Config.Quality)
	assert.Equal(t, 2048, e.Config.MaxDimension)
}

func TestRegisterDefaults(t *testing.T) {
	e := New(nil)
	e.RegisterDefaults()

	// Image processors should always be registered.
	imageProcs := []string{"resize", "thumbnail", "webp", "optimize", "convert", "watermark", "metadata"}
	for _, name := range imageProcs {
		p, ok := e.Registry.Get(name)
		assert.True(t, ok, "expected processor %q to be registered", name)
		if ok {
			assert.Equal(t, name, p.Name())
		}
	}

	// PDF processor should always be registered.
	p, ok := e.Registry.Get("pdf_thumbnail")
	assert.True(t, ok, "expected pdf_thumbnail to be registered")
	if ok {
		assert.Equal(t, "pdf_thumbnail", p.Name())
	}

	// Video processors are optional (depend on ffmpeg).
	if FFmpegAvailable() {
		_, ok := e.Registry.Get("video_thumbnail")
		assert.True(t, ok, "ffmpeg available but video_thumbnail not registered")
		_, ok = e.Registry.Get("video_transcode")
		assert.True(t, ok, "ffmpeg available but video_transcode not registered")
	}
}

func TestListProcessors(t *testing.T) {
	e := New(nil)
	e.RegisterDefaults()

	list := e.ListProcessors()
	assert.GreaterOrEqual(t, len(list), 8) // at least 7 image + 1 pdf

	names := make(map[string]bool)
	for _, info := range list {
		names[info.Name] = true
		assert.NotEmpty(t, info.SupportedTypes, "processor %q should have supported types", info.Name)
	}

	assert.True(t, names["resize"])
	assert.True(t, names["thumbnail"])
	assert.True(t, names["metadata"])
}

func TestDetectContentType(t *testing.T) {
	dir := t.TempDir()
	jpegPath := createTestJPEG(t, dir, "test.jpg")

	ct, err := DetectContentType(jpegPath)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", ct)
}

func TestDetectContentTypeText(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(txtPath, []byte("hello world"), 0644))

	ct, err := DetectContentType(txtPath)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(ct, "text/plain"), "expected text/plain, got %s", ct)
}

func TestDetectContentTypeNotFound(t *testing.T) {
	_, err := DetectContentType("/nonexistent/file.bin")
	assert.Error(t, err)
}

// --- GenerateOutputPath tests ---

func TestGenerateOutputPath_Resize(t *testing.T) {
	p := GenerateOutputPath("/tmp/photo.jpg", "resize", nil)
	assert.Equal(t, "/tmp/photo_resized.jpg", p)
}

func TestGenerateOutputPath_Thumbnail(t *testing.T) {
	p := GenerateOutputPath("/tmp/photo.jpg", "thumbnail", nil)
	assert.Equal(t, "/tmp/photo_thumb.jpg", p)
}

func TestGenerateOutputPath_WebP(t *testing.T) {
	p := GenerateOutputPath("/tmp/photo.jpg", "webp", nil)
	assert.Equal(t, "/tmp/photo.webp", p)
}

func TestGenerateOutputPath_Optimize(t *testing.T) {
	p := GenerateOutputPath("/tmp/photo.png", "optimize", nil)
	assert.Equal(t, "/tmp/photo_optimized.png", p)
}

func TestGenerateOutputPath_Convert(t *testing.T) {
	opts := &processor.Options{Format: "webp"}
	p := GenerateOutputPath("/tmp/photo.jpg", "convert", opts)
	assert.Equal(t, "/tmp/photo.webp", p)
}

func TestGenerateOutputPath_ConvertDefaultFormat(t *testing.T) {
	p := GenerateOutputPath("/tmp/photo.jpg", "convert", nil)
	assert.Equal(t, "/tmp/photo.png", p)
}

func TestGenerateOutputPath_Watermark(t *testing.T) {
	p := GenerateOutputPath("/tmp/photo.jpg", "watermark", nil)
	assert.Equal(t, "/tmp/photo_watermarked.jpg", p)
}

func TestGenerateOutputPath_Metadata(t *testing.T) {
	p := GenerateOutputPath("/tmp/photo.jpg", "metadata", nil)
	assert.Equal(t, "", p)
}

func TestGenerateOutputPath_PDFThumbnail(t *testing.T) {
	p := GenerateOutputPath("/tmp/document.pdf", "pdf_thumbnail", nil)
	assert.Equal(t, "/tmp/document_thumb.png", p)
}

func TestGenerateOutputPath_VideoThumbnail(t *testing.T) {
	p := GenerateOutputPath("/tmp/video.mp4", "video_thumbnail", nil)
	assert.Equal(t, "/tmp/video_thumb.jpg", p)
}

func TestGenerateOutputPath_VideoTranscode(t *testing.T) {
	p := GenerateOutputPath("/tmp/video.mp4", "video_transcode", nil)
	assert.Equal(t, "/tmp/video_transcoded.mp4", p)
}

func TestGenerateOutputPath_UnknownProcessor(t *testing.T) {
	p := GenerateOutputPath("/tmp/file.dat", "custom_proc", nil)
	assert.Equal(t, "/tmp/file_processed.dat", p)
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()

	// Create a file so the unique logic kicks in.
	first := filepath.Join(dir, "photo_resized.jpg")
	require.NoError(t, os.WriteFile(first, []byte("x"), 0644))

	result := GenerateOutputPath(filepath.Join(dir, "photo.jpg"), "resize", nil)
	assert.Equal(t, filepath.Join(dir, "photo_resized_1.jpg"), result)

	// Create _1 too.
	require.NoError(t, os.WriteFile(result, []byte("x"), 0644))

	result2 := GenerateOutputPath(filepath.Join(dir, "photo.jpg"), "resize", nil)
	assert.Equal(t, filepath.Join(dir, "photo_resized_2.jpg"), result2)
}

// --- Process tests ---

func TestProcess_Resize(t *testing.T) {
	dir := t.TempDir()
	jpegPath := createTestJPEG(t, dir, "input.jpg")

	e := New(nil)
	e.RegisterDefaults()

	req := &Request{
		InputPath: jpegPath,
		Processor: "resize",
		Options: &processor.Options{
			Width:  32,
			Height: 32,
		},
	}

	res, err := e.Process(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, jpegPath, res.InputPath)
	assert.NotEmpty(t, res.OutputPath)
	assert.Greater(t, res.InputSize, int64(0))
	assert.Greater(t, res.OutputSize, int64(0))
	assert.Greater(t, res.Duration, 0*time.Nanosecond)

	// Output file should exist.
	_, err = os.Stat(res.OutputPath)
	assert.NoError(t, err)
}

func TestProcess_ExplicitOutputPath(t *testing.T) {
	dir := t.TempDir()
	jpegPath := createTestJPEG(t, dir, "input.jpg")
	outPath := filepath.Join(dir, "custom_output.jpg")

	e := New(nil)
	e.RegisterDefaults()

	req := &Request{
		InputPath:  jpegPath,
		OutputPath: outPath,
		Processor:  "resize",
		Options:    &processor.Options{Width: 32, Height: 32},
	}

	res, err := e.Process(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, outPath, res.OutputPath)

	_, err = os.Stat(outPath)
	assert.NoError(t, err)
}

func TestProcess_Metadata(t *testing.T) {
	dir := t.TempDir()
	jpegPath := createTestJPEG(t, dir, "input.jpg")

	e := New(nil)
	e.RegisterDefaults()

	req := &Request{
		InputPath: jpegPath,
		Processor: "metadata",
	}

	res, err := e.Process(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.OutputPath)
	assert.Greater(t, res.Metadata.Width, 0)
	assert.Greater(t, res.Metadata.Height, 0)
}

func TestProcess_UnknownProcessor(t *testing.T) {
	e := New(nil)
	e.RegisterDefaults()

	req := &Request{
		InputPath: "/tmp/fake.jpg",
		Processor: "nonexistent",
	}

	_, err := e.Process(context.Background(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

func TestProcess_NilRequest(t *testing.T) {
	e := New(nil)
	_, err := e.Process(context.Background(), nil)
	assert.Error(t, err)
}

func TestProcess_MissingInput(t *testing.T) {
	e := New(nil)
	e.RegisterDefaults()

	req := &Request{
		InputPath: "/nonexistent/file.jpg",
		Processor: "resize",
	}

	_, err := e.Process(context.Background(), req)
	assert.Error(t, err)
}

// --- ProcessBatch tests ---

func TestProcessBatch(t *testing.T) {
	dir := t.TempDir()
	jpeg1 := createTestJPEG(t, dir, "a.jpg")
	jpeg2 := createTestJPEG(t, dir, "b.jpg")

	e := New(nil)
	e.RegisterDefaults()

	reqs := []*Request{
		{InputPath: jpeg1, Processor: "resize", Options: &processor.Options{Width: 16, Height: 16}},
		{InputPath: jpeg2, Processor: "thumbnail", Options: &processor.Options{Width: 16, Height: 16}},
	}

	results, errs := e.ProcessBatch(context.Background(), reqs, 2)
	require.Len(t, results, 2)
	require.Len(t, errs, 2)

	for i := range results {
		assert.NoError(t, errs[i], "request %d", i)
		assert.NotNil(t, results[i], "request %d", i)
		assert.NotEmpty(t, results[i].OutputPath, "request %d", i)
	}
}

func TestProcessBatch_PartialFailure(t *testing.T) {
	dir := t.TempDir()
	jpegPath := createTestJPEG(t, dir, "good.jpg")

	e := New(nil)
	e.RegisterDefaults()

	reqs := []*Request{
		{InputPath: jpegPath, Processor: "resize", Options: &processor.Options{Width: 16, Height: 16}},
		{InputPath: "/nonexistent/bad.jpg", Processor: "resize"},
	}

	results, errs := e.ProcessBatch(context.Background(), reqs, 2)
	require.Len(t, results, 2)
	require.Len(t, errs, 2)

	assert.NoError(t, errs[0])
	assert.NotNil(t, results[0])

	assert.Error(t, errs[1])
	assert.Nil(t, results[1])
}

func TestProcessBatch_ZeroConcurrency(t *testing.T) {
	// concurrency < 1 should be treated as 1 (no panic).
	dir := t.TempDir()
	jpegPath := createTestJPEG(t, dir, "c.jpg")

	e := New(nil)
	e.RegisterDefaults()

	reqs := []*Request{
		{InputPath: jpegPath, Processor: "metadata"},
	}

	results, errs := e.ProcessBatch(context.Background(), reqs, 0)
	assert.Len(t, results, 1)
	assert.NoError(t, errs[0])
}
