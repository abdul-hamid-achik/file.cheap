package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor/video"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type videoThumbnailInput struct {
	Path            string `json:"path" jsonschema:"Absolute path to the video file"`
	PositionPercent int    `json:"position_percent,omitempty" jsonschema:"Position in video as percentage 0-100 (default 10)"`
	Width           int    `json:"width,omitempty" jsonschema:"Thumbnail width in pixels"`
	Height          int    `json:"height,omitempty" jsonschema:"Thumbnail height in pixels"`
	OutputPath      string `json:"output_path,omitempty" jsonschema:"Output file path (auto-generated if omitted)"`
}

type transcodeVideoInput struct {
	Path          string `json:"path" jsonschema:"Absolute path to the video file"`
	Format        string `json:"format,omitempty" jsonschema:"Output format: mp4 or webm"`
	Quality       int    `json:"quality,omitempty" jsonschema:"Video quality 1-100"`
	MaxResolution int    `json:"max_resolution,omitempty" jsonschema:"Maximum output height in pixels: 480, 720, 1080, or 2160"`
	Preset        string `json:"preset,omitempty" jsonschema:"Encoding preset: ultrafast, veryfast, fast, medium, or slow"`
	OutputPath    string `json:"output_path,omitempty" jsonschema:"Output file path (auto-generated if omitted)"`
}

type videoWatermarkInput struct {
	Path       string  `json:"path" jsonschema:"Absolute path to the video file"`
	Text       string  `json:"text" jsonschema:"Watermark text to overlay"`
	Position   string  `json:"position,omitempty" jsonschema:"Watermark position: center, bottom-right, bottom-left, top-right, or top-left"`
	Opacity    float64 `json:"opacity,omitempty" jsonschema:"Watermark opacity 0.0-1.0"`
	OutputPath string  `json:"output_path,omitempty" jsonschema:"Output file path (auto-generated if omitted)"`
}

type generateHLSInput struct {
	Path            string `json:"path" jsonschema:"Absolute path to the video file"`
	SegmentDuration int    `json:"segment_duration,omitempty" jsonschema:"Segment duration in seconds (default 6)"`
	Quality         int    `json:"quality,omitempty" jsonschema:"Video quality 1-100"`
	OutputDir       string `json:"output_dir,omitempty" jsonschema:"Output directory for HLS files (auto-generated if omitted)"`
}

func registerVideoTools(srv *mcp.Server, eng *engine.Engine) {
	falseVal := false

	// fc_video_thumbnail
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_video_thumbnail",
		Description: "Extract a thumbnail frame from a video. Requires ffmpeg.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in videoThumbnailInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		if !engine.FFmpegAvailable() {
			r, _ := toolError("ffmpeg is not installed; run 'fc doctor' to check dependencies")
			return r, nil, nil
		}
		pos := in.PositionPercent
		if pos == 0 {
			pos = 10
		}
		return processAndRespond(ctx, eng, &engine.Request{
			InputPath:  absPath(in.Path),
			OutputPath: in.OutputPath,
			Processor:  "video_thumbnail",
			Options: &processor.Options{
				Quality: pos,
				Width:   in.Width,
				Height:  in.Height,
			},
		})
	})

	// fc_transcode_video
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_transcode_video",
		Description: "Transcode a video to a different format or quality. Requires ffmpeg.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in transcodeVideoInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		if !engine.FFmpegAvailable() {
			r, _ := toolError("ffmpeg is not installed; run 'fc doctor' to check dependencies")
			return r, nil, nil
		}
		format := in.Format
		if format == "" {
			format = "mp4"
		}
		return processAndRespond(ctx, eng, &engine.Request{
			InputPath:  absPath(in.Path),
			OutputPath: in.OutputPath,
			Processor:  "video_transcode",
			Options: &processor.Options{
				Format:  format,
				Quality: in.Quality,
			},
		})
	})

	// fc_video_watermark
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_video_watermark",
		Description: "Add a text watermark to a video. Requires ffmpeg.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in videoWatermarkInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		if !engine.FFmpegAvailable() {
			r, _ := toolError("ffmpeg is not installed; run 'fc doctor' to check dependencies")
			return r, nil, nil
		}
		opacity := in.Opacity
		if opacity <= 0 {
			opacity = 0.5
		}
		position := in.Position
		if position == "" {
			position = "bottom-right"
		}

		// Video watermark uses the FFmpegProcessor directly since it's not
		// exposed as a separate engine processor.
		vcfg := video.DefaultVideoConfig()
		vcfg.Config = eng.Config
		proc, err := video.NewFFmpegProcessor(vcfg)
		if err != nil {
			r, _ := toolError("ffmpeg unavailable: %v", err)
			return r, nil, nil
		}

		f, err := os.Open(absPath(in.Path))
		if err != nil {
			r, _ := toolError("cannot open file: %v", err)
			return r, nil, nil
		}
		defer f.Close() //nolint:errcheck // read-only

		result, err := proc.AddWatermark(ctx, f, in.Text, position, opacity)
		if err != nil {
			r, _ := toolError("video watermark failed: %v", err)
			return r, nil, nil
		}

		// Write the result to the output path.
		outPath := in.OutputPath
		if outPath == "" {
			dir := filepath.Dir(absPath(in.Path))
			base := filepath.Base(in.Path)
			ext := filepath.Ext(base)
			name := base[:len(base)-len(ext)]
			outPath = filepath.Join(dir, name+"_watermarked.mp4")
		}

		outFile, err := os.Create(outPath)
		if err != nil {
			r, _ := toolError("cannot create output: %v", err)
			return r, nil, nil
		}
		defer outFile.Close() //nolint:errcheck // write errors caught by io.Copy

		written, err := io.Copy(outFile, result.Data)
		if err != nil {
			r, _ := toolError("write failed: %v", err)
			return r, nil, nil
		}

		fi, _ := os.Stat(absPath(in.Path))
		inputSize := int64(0)
		if fi != nil {
			inputSize = fi.Size()
		}

		summary := map[string]any{
			"input_path":  absPath(in.Path),
			"output_path": outPath,
			"input_size":  inputSize,
			"output_size": written,
			"format":      "mp4",
		}
		b, _ := json.MarshalIndent(summary, "", "  ")
		return textResult(string(b)), nil, nil
	})

	// fc_generate_hls
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_generate_hls",
		Description: "Generate HLS (HTTP Live Streaming) segments and playlist from a video. Requires ffmpeg.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in generateHLSInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		if !engine.FFmpegAvailable() {
			r, _ := toolError("ffmpeg is not installed; run 'fc doctor' to check dependencies")
			return r, nil, nil
		}

		vcfg := video.DefaultVideoConfig()
		vcfg.Config = eng.Config
		proc, err := video.NewFFmpegProcessor(vcfg)
		if err != nil {
			r, _ := toolError("ffmpeg unavailable: %v", err)
			return r, nil, nil
		}

		f, err := os.Open(absPath(in.Path))
		if err != nil {
			r, _ := toolError("cannot open file: %v", err)
			return r, nil, nil
		}
		defer f.Close() //nolint:errcheck // read-only

		segDur := in.SegmentDuration
		if segDur <= 0 {
			segDur = 6
		}

		crf := 28
		if in.Quality > 0 {
			crf = 51 - (in.Quality * 51 / 100)
		}

		vopts := &video.VideoOptions{
			Options:            &processor.Options{},
			Preset:             vcfg.DefaultPreset,
			CRF:                crf,
			HLSSegmentDuration: segDur,
		}

		hlsResult, err := proc.GenerateHLS(ctx, vopts, f)
		if err != nil {
			r, _ := toolError("HLS generation failed: %v", err)
			return r, nil, nil
		}

		// If an output directory is specified, move files there.
		outputDir := in.OutputDir
		if outputDir != "" {
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				r, _ := toolError("cannot create output directory: %v", err)
				return r, nil, nil
			}

			// Move manifest.
			newManifest := filepath.Join(outputDir, filepath.Base(hlsResult.ManifestPath))
			if err := moveFile(hlsResult.ManifestPath, newManifest); err != nil {
				r, _ := toolError("failed to move manifest: %v", err)
				return r, nil, nil
			}
			hlsResult.ManifestPath = newManifest

			// Move segments.
			newSegments := make([]string, len(hlsResult.SegmentPaths))
			for i, seg := range hlsResult.SegmentPaths {
				dst := filepath.Join(outputDir, filepath.Base(seg))
				if err := moveFile(seg, dst); err != nil {
					r, _ := toolError("failed to move segment: %v", err)
					return r, nil, nil
				}
				newSegments[i] = dst
			}
			hlsResult.SegmentPaths = newSegments
		}

		summary := map[string]any{
			"manifest_path":  hlsResult.ManifestPath,
			"segment_count":  hlsResult.SegmentCount,
			"total_duration": hlsResult.TotalDuration,
			"resolutions":    hlsResult.Resolutions,
			"segments":       hlsResult.SegmentPaths,
		}
		b, _ := json.MarshalIndent(summary, "", "  ")
		return textResult(string(b)), nil, nil
	})
}

// moveFile copies src to dst then removes src.
func moveFile(src, dst string) error {
	// Try rename first (same filesystem).
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// Fall back to copy + remove.
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close() //nolint:errcheck // read-only
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close() //nolint:errcheck // write errors caught by io.Copy
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return os.Remove(src)
}
