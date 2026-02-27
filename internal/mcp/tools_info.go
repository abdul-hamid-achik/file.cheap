package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// -- Input structs --

type fileInfoInput struct {
	Path string `json:"path" jsonschema:"Absolute path to the file to inspect"`
}

type videoMetadataInput struct {
	Path string `json:"path" jsonschema:"Absolute path to the video file"`
}

// -- Response structs --

type fileInfoResult struct {
	Path        string `json:"path"`
	Filename    string `json:"filename"`
	Extension   string `json:"extension"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	SizeHuman   string `json:"size_human"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

type videoMetadataResult struct {
	DurationSeconds float64 `json:"duration_seconds"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	BitrateBps      int64   `json:"bitrate_bps"`
	VideoCodec      string  `json:"video_codec"`
	AudioCodec      string  `json:"audio_codec"`
	FrameRate       float64 `json:"frame_rate"`
	FileSizeBytes   int64   `json:"file_size_bytes"`
	Container       string  `json:"container"`
	HasAudio        bool    `json:"has_audio"`
}

// ffprobeOutput represents the JSON output from ffprobe.
type ffprobeOutput struct {
	Streams []struct {
		CodecType  string `json:"codec_type"`
		CodecName  string `json:"codec_name"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		RFrameRate string `json:"r_frame_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		Size     string `json:"size"`
		BitRate  string `json:"bit_rate"`
		Name     string `json:"format_name"`
	} `json:"format"`
}

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func registerInfoTools(srv *mcp.Server, eng *engine.Engine) {
	falseVal := false

	// fc_file_info
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_file_info",
		Description: "Detect file type, MIME content type, size, and basic dimensions without processing",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in fileInfoInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}

		abs := absPath(in.Path)

		stat, err := os.Stat(abs)
		if err != nil {
			r, _ := toolError("cannot stat file: %v", err)
			return r, nil, nil
		}

		contentType, err := engine.DetectContentType(abs)
		if err != nil {
			r, _ := toolError("cannot detect content type: %v", err)
			return r, nil, nil
		}

		info := fileInfoResult{
			Path:        abs,
			Filename:    filepath.Base(abs),
			Extension:   strings.TrimPrefix(filepath.Ext(abs), "."),
			ContentType: contentType,
			SizeBytes:   stat.Size(),
			SizeHuman:   formatSize(stat.Size()),
		}

		// Try to get image dimensions if it's an image.
		if strings.HasPrefix(contentType, "image/") {
			if f, err := os.Open(abs); err == nil {
				if cfg, _, err := image.DecodeConfig(f); err == nil {
					info.Width = cfg.Width
					info.Height = cfg.Height
				}
				f.Close()
			}
		}

		data, _ := json.MarshalIndent(info, "", "  ")
		return textResult(string(data)), nil, nil
	})

	// fc_video_metadata
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_video_metadata",
		Description: "Extract video metadata (duration, resolution, codecs, bitrate) using ffprobe",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in videoMetadataInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}

		if !engine.FFmpegAvailable() {
			r, _ := toolError("ffmpeg is not installed; run 'fc doctor' to check dependencies")
			return r, nil, nil
		}

		abs := absPath(in.Path)

		cmd := exec.CommandContext(ctx, "ffprobe",
			"-v", "quiet",
			"-print_format", "json",
			"-show_format",
			"-show_streams",
			abs,
		)
		output, err := cmd.Output()
		if err != nil {
			r, _ := toolError("ffprobe failed: %v", err)
			return r, nil, nil
		}

		var probe ffprobeOutput
		if err := json.Unmarshal(output, &probe); err != nil {
			r, _ := toolError("failed to parse ffprobe output: %v", err)
			return r, nil, nil
		}

		result := videoMetadataResult{}

		// Parse format-level fields.
		if probe.Format.Duration != "" {
			if d, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
				result.DurationSeconds = d
			}
		}
		if probe.Format.Size != "" {
			if s, err := strconv.ParseInt(probe.Format.Size, 10, 64); err == nil {
				result.FileSizeBytes = s
			}
		}
		if probe.Format.BitRate != "" {
			if b, err := strconv.ParseInt(probe.Format.BitRate, 10, 64); err == nil {
				result.BitrateBps = b
			}
		}
		result.Container = strings.Split(probe.Format.Name, ",")[0]

		// Parse stream-level fields.
		for _, stream := range probe.Streams {
			switch stream.CodecType {
			case "video":
				result.VideoCodec = stream.CodecName
				result.Width = stream.Width
				result.Height = stream.Height
				if stream.RFrameRate != "" {
					parts := strings.Split(stream.RFrameRate, "/")
					if len(parts) == 2 {
						num, _ := strconv.ParseFloat(parts[0], 64)
						den, _ := strconv.ParseFloat(parts[1], 64)
						if den > 0 {
							result.FrameRate = num / den
						}
					}
				}
			case "audio":
				result.AudioCodec = stream.CodecName
				result.HasAudio = true
			}
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		return textResult(string(data)), nil, nil
	})
}
