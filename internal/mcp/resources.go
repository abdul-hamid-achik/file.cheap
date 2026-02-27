package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func registerResources(srv *mcp.Server, eng *engine.Engine) {
	// fcheap://supported-formats — static listing of all supported file formats.
	srv.AddResource(&mcp.Resource{
		Name:     "supported-formats",
		Title:    "Supported File Formats",
		MIMEType: "application/json",
		URI:      "fcheap://supported-formats",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		formats := map[string]any{
			"image": map[string]any{
				"input":      []string{"jpeg", "png", "gif", "webp", "bmp"},
				"output":     []string{"jpeg", "png", "gif", "webp", "bmp", "tiff"},
				"processors": []string{"resize", "thumbnail", "webp", "optimize", "convert", "watermark", "metadata"},
			},
			"pdf": map[string]any{
				"input":      []string{"pdf"},
				"output":     []string{"png", "jpeg"},
				"processors": []string{"pdf_thumbnail"},
			},
			"video": map[string]any{
				"input":      []string{"mp4", "webm", "avi", "mov", "mkv", "mpeg", "ogg", "3gpp"},
				"output":     []string{"mp4", "webm", "hls"},
				"processors": []string{"video_thumbnail", "video_transcode"},
			},
		}

		data, _ := json.MarshalIndent(formats, "", "  ")
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
			},
		}, nil
	})

	// fcheap://capabilities — dynamic listing of registered processors and dependency status.
	srv.AddResource(&mcp.Resource{
		Name:     "capabilities",
		Title:    "Processing Capabilities",
		MIMEType: "application/json",
		URI:      "fcheap://capabilities",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		processors := eng.ListProcessors()

		type depStatus struct {
			Name      string `json:"name"`
			Available bool   `json:"available"`
			Purpose   string `json:"purpose"`
		}

		deps := []depStatus{
			{"ffmpeg", checkBinary("ffmpeg"), "Video processing"},
			{"ffprobe", checkBinary("ffprobe"), "Video metadata extraction"},
			{"pdftoppm", checkBinary("pdftoppm"), "PDF thumbnails (poppler-utils)"},
			{"pdfinfo", checkBinary("pdfinfo"), "PDF page counting (poppler-utils)"},
			{"mutool", checkBinary("mutool"), "PDF thumbnails (mupdf alternative)"},
			{"cwebp", checkBinary("cwebp"), "WebP conversion (optional, pure Go fallback)"},
		}

		out := map[string]any{
			"processors":   processors,
			"dependencies": deps,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
			},
		}, nil
	})

	// fcheap://file/{path} — file info resource template.
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "file-info",
		Title:       "File Information",
		URITemplate: "fcheap://file/{path}",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		path := req.Params.URI
		// Strip the URI scheme prefix to get the file path.
		const prefix = "fcheap://file/"
		if len(path) > len(prefix) {
			path = path[len(prefix):]
		}

		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}

		stat, err := os.Stat(abs)
		if err != nil {
			return nil, err
		}

		contentType, err := engine.DetectContentType(abs)
		if err != nil {
			contentType = "application/octet-stream"
		}

		info := map[string]any{
			"path":         abs,
			"filename":     filepath.Base(abs),
			"extension":    filepath.Ext(abs),
			"content_type": contentType,
			"size_bytes":   stat.Size(),
			"size_human":   formatSize(stat.Size()),
		}

		data, _ := json.MarshalIndent(info, "", "  ")
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{
				{URI: req.Params.URI, MIMEType: "application/json", Text: string(data)},
			},
		}, nil
	})
}

