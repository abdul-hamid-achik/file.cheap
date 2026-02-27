package mcp

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

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/presets"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestJPEG200 writes a solid-color 200x200 JPEG into dir and returns its path.
// This is distinct from server_test.go's createTestEngine helper.
func createTestJPEG200(t *testing.T, dir, name string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := range 200 {
		for x := range 200 {
			img.Set(x, y, color.RGBA{R: 255, G: 100, B: 50, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0644))
	return path
}

// ---------------------------------------------------------------------------
// TestFileInfoTool_Integration
// ---------------------------------------------------------------------------

func TestFileInfoTool_Integration(t *testing.T) {
	dir := t.TempDir()
	jpegPath := createTestJPEG200(t, dir, "photo.jpg")

	stat, err := os.Stat(jpegPath)
	require.NoError(t, err)
	assert.Greater(t, stat.Size(), int64(0))

	ct, err := engine.DetectContentType(jpegPath)
	require.NoError(t, err)
	assert.Equal(t, "image/jpeg", ct)
}

// ---------------------------------------------------------------------------
// TestPresetApply_Thumbnail
// ---------------------------------------------------------------------------

func TestPresetApply_Thumbnail(t *testing.T) {
	dir := t.TempDir()
	jpegPath := createTestJPEG200(t, dir, "thumb_input.jpg")

	p, ok := presets.Get("thumbnail")
	require.True(t, ok, "thumbnail preset must exist")
	assert.True(t, p.Crop, "thumbnail preset must have Crop=true")
	assert.Equal(t, 300, p.Width)
	assert.Equal(t, 300, p.Height)

	eng := createTestEngine(t)
	outPath := filepath.Join(dir, "thumb_output.jpg")

	res, err := eng.Process(context.Background(), &engine.Request{
		InputPath:  jpegPath,
		OutputPath: outPath,
		Processor:  "thumbnail",
		Options: &processor.Options{
			Width:    p.Width,
			Height:   p.Height,
			Quality:  p.Quality,
			Position: "center",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, outPath, res.OutputPath)

	_, err = os.Stat(outPath)
	assert.NoError(t, err, "output file must exist")
}

// ---------------------------------------------------------------------------
// TestPresetApply_Resize
// ---------------------------------------------------------------------------

func TestPresetApply_Resize(t *testing.T) {
	dir := t.TempDir()
	jpegPath := createTestJPEG200(t, dir, "resize_input.jpg")

	p, ok := presets.Get("sm")
	require.True(t, ok, "sm preset must exist")
	assert.False(t, p.Crop, "sm preset must have Crop=false")
	assert.Equal(t, 640, p.Width)

	eng := createTestEngine(t)
	outPath := filepath.Join(dir, "resize_output.jpg")

	res, err := eng.Process(context.Background(), &engine.Request{
		InputPath:  jpegPath,
		OutputPath: outPath,
		Processor:  "resize",
		Options: &processor.Options{
			Width:   p.Width,
			Height:  p.Height,
			Quality: p.Quality,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, outPath, res.OutputPath)

	_, err = os.Stat(outPath)
	assert.NoError(t, err, "output file must exist")
}

// ---------------------------------------------------------------------------
// TestPresetList
// ---------------------------------------------------------------------------

func TestPresetList(t *testing.T) {
	assert.GreaterOrEqual(t, len(presets.All), 14, "should have at least 14 presets")

	og, ok := presets.Get("og")
	require.True(t, ok)
	assert.Equal(t, 1200, og.Width)
	assert.Equal(t, 630, og.Height)

	thumb, ok := presets.Get("thumbnail")
	require.True(t, ok)
	assert.True(t, thumb.Crop)

	// Verify all well-known presets exist.
	expected := []string{
		"thumbnail", "sm", "md", "lg", "xl",
		"og", "twitter", "instagram_square",
		"instagram_portrait", "instagram_story",
		"pdf_thumbnail", "pdf_sm", "pdf_md", "pdf_lg",
	}
	for _, name := range expected {
		_, exists := presets.Get(name)
		assert.True(t, exists, "preset %q should exist", name)
	}
}

// ---------------------------------------------------------------------------
// TestPipeline_ResizeThenOptimize
// ---------------------------------------------------------------------------

func TestPipeline_ResizeThenOptimize(t *testing.T) {
	dir := t.TempDir()
	jpegPath := createTestJPEG200(t, dir, "pipeline_input.jpg")

	eng := createTestEngine(t)
	ctx := context.Background()

	// Step 1: resize to 100x100
	step1Out := filepath.Join(dir, "step1_resized.jpg")
	res1, err := eng.Process(ctx, &engine.Request{
		InputPath:  jpegPath,
		OutputPath: step1Out,
		Processor:  "resize",
		Options:    &processor.Options{Width: 100, Height: 100},
	})
	require.NoError(t, err)
	require.NotNil(t, res1)
	assert.Equal(t, step1Out, res1.OutputPath)

	// Step 2: optimize with quality 80
	step2Out := filepath.Join(dir, "step2_optimized.jpg")
	res2, err := eng.Process(ctx, &engine.Request{
		InputPath:  step1Out,
		OutputPath: step2Out,
		Processor:  "optimize",
		Options:    &processor.Options{Quality: 80},
	})
	require.NoError(t, err)
	require.NotNil(t, res2)
	assert.Equal(t, step2Out, res2.OutputPath)

	_, err = os.Stat(step2Out)
	assert.NoError(t, err, "final pipeline output must exist")

	// Clean up intermediate.
	os.Remove(step1Out)
}

// ---------------------------------------------------------------------------
// TestPipeline_ResizeThenWebP
// ---------------------------------------------------------------------------

func TestPipeline_ResizeThenWebP(t *testing.T) {
	dir := t.TempDir()
	jpegPath := createTestJPEG200(t, dir, "pipeline_webp_input.jpg")

	eng := createTestEngine(t)
	ctx := context.Background()

	// Step 1: resize to 50x50
	step1Out := filepath.Join(dir, "step1_small.jpg")
	res1, err := eng.Process(ctx, &engine.Request{
		InputPath:  jpegPath,
		OutputPath: step1Out,
		Processor:  "resize",
		Options:    &processor.Options{Width: 50, Height: 50},
	})
	require.NoError(t, err)
	require.NotNil(t, res1)

	// Step 2: convert to webp
	step2Out := filepath.Join(dir, "step2_output.webp")
	res2, err := eng.Process(ctx, &engine.Request{
		InputPath:  step1Out,
		OutputPath: step2Out,
		Processor:  "webp",
	})
	require.NoError(t, err)
	require.NotNil(t, res2)

	assert.True(t, strings.HasSuffix(res2.OutputPath, ".webp"),
		"output should have .webp extension, got %s", res2.OutputPath)

	_, err = os.Stat(step2Out)
	assert.NoError(t, err, "webp output must exist")

	// Clean up intermediate.
	os.Remove(step1Out)
}

// ---------------------------------------------------------------------------
// TestBatchProcess_Integration
// ---------------------------------------------------------------------------

func TestBatchProcess_Integration(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 3)
	for i := range 3 {
		paths[i] = createTestJPEG200(t, dir, "batch_"+string(rune('a'+i))+".jpg")
	}

	eng := createTestEngine(t)

	reqs := make([]*engine.Request, len(paths))
	for i, p := range paths {
		reqs[i] = &engine.Request{
			InputPath: p,
			Processor: "resize",
			Options:   &processor.Options{Width: 32, Height: 32},
		}
	}

	results, errs := eng.ProcessBatch(context.Background(), reqs, 2)
	require.Len(t, results, 3)
	require.Len(t, errs, 3)

	for i := range results {
		assert.NoError(t, errs[i], "batch item %d should succeed", i)
		require.NotNil(t, results[i], "batch item %d should have a result", i)
		assert.NotEmpty(t, results[i].OutputPath, "batch item %d should have an output path", i)

		_, err := os.Stat(results[i].OutputPath)
		assert.NoError(t, err, "batch item %d output file should exist", i)
	}
}

// ---------------------------------------------------------------------------
// TestImageMetadata_Integration
// ---------------------------------------------------------------------------

func TestImageMetadata_Integration(t *testing.T) {
	dir := t.TempDir()
	jpegPath := createTestJPEG200(t, dir, "meta_input.jpg")

	eng := createTestEngine(t)

	res, err := eng.Process(context.Background(), &engine.Request{
		InputPath: jpegPath,
		Processor: "metadata",
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, 200, res.Metadata.Width, "metadata width should be 200")
	assert.Equal(t, 200, res.Metadata.Height, "metadata height should be 200")
	assert.Empty(t, res.OutputPath, "metadata processor should not produce an output file")
}

// ---------------------------------------------------------------------------
// TestValidOperations
// ---------------------------------------------------------------------------

func TestValidOperations(t *testing.T) {
	eng := createTestEngine(t)

	// These are the processor names referenced by the MCP tools.
	alwaysAvailable := []string{
		"resize",
		"thumbnail",
		"webp",
		"optimize",
		"convert",
		"watermark",
		"metadata",
		"pdf_thumbnail",
	}

	for _, name := range alwaysAvailable {
		t.Run(name, func(t *testing.T) {
			p, ok := eng.Registry.Get(name)
			assert.True(t, ok, "processor %q must be registered", name)
			if ok {
				assert.Equal(t, name, p.Name())
				assert.NotEmpty(t, p.SupportedTypes(), "processor %q must declare supported types", name)
			}
		})
	}

	// Video processors depend on ffmpeg — only check if available.
	if engine.FFmpegAvailable() {
		videoProcs := []string{"video_thumbnail", "video_transcode"}
		for _, name := range videoProcs {
			t.Run(name, func(t *testing.T) {
				p, ok := eng.Registry.Get(name)
				assert.True(t, ok, "processor %q must be registered when ffmpeg is available", name)
				if ok {
					assert.Equal(t, name, p.Name())
				}
			})
		}
	}
}
