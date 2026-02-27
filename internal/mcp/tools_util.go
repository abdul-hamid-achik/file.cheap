package mcp

import (
	"context"
	"encoding/json"
	"os/exec"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type batchProcessInput struct {
	Paths     []string `json:"paths" jsonschema:"List of absolute file paths to process"`
	Operation string   `json:"operation" jsonschema:"Processing operation: resize, thumbnail, webp, optimize, convert, watermark, pdf_thumbnail, video_thumbnail, or video_transcode"`
	Width     int      `json:"width,omitempty" jsonschema:"Width in pixels"`
	Height    int      `json:"height,omitempty" jsonschema:"Height in pixels"`
	Quality   int      `json:"quality,omitempty" jsonschema:"Quality 1-100"`
	Format    string   `json:"format,omitempty" jsonschema:"Output format (for convert/transcode)"`
	Text      string   `json:"text,omitempty" jsonschema:"Text (for watermark operations)"`
}

type listCapabilitiesInput struct{}

func registerUtilTools(srv *mcp.Server, eng *engine.Engine) {
	falseVal := false

	// fc_batch_process
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_batch_process",
		Description: "Apply a processing operation to multiple files at once",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in batchProcessInput) (*mcp.CallToolResult, any, error) {
		if len(in.Paths) == 0 {
			r, _ := toolError("paths must not be empty")
			return r, nil, nil
		}

		opts := &processor.Options{
			Width:   in.Width,
			Height:  in.Height,
			Quality: in.Quality,
			Format:  in.Format,
		}
		if in.Text != "" {
			opts.VariantType = in.Text
		}

		reqs := make([]*engine.Request, len(in.Paths))
		for i, p := range in.Paths {
			reqs[i] = &engine.Request{
				InputPath: absPath(p),
				Processor: in.Operation,
				Options:   opts,
			}
		}

		results, errs := eng.ProcessBatch(ctx, reqs, 4)

		type batchItem struct {
			Path   string         `json:"path"`
			Result *engine.Result `json:"result,omitempty"`
			Error  string         `json:"error,omitempty"`
		}

		items := make([]batchItem, len(in.Paths))
		successCount := 0
		for i, p := range in.Paths {
			item := batchItem{Path: p}
			if errs[i] != nil {
				item.Error = errs[i].Error()
			} else {
				item.Result = results[i]
				successCount++
			}
			items[i] = item
		}

		out := map[string]any{
			"total":      len(in.Paths),
			"successful": successCount,
			"failed":     len(in.Paths) - successCount,
			"results":    items,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		return textResult(string(data)), nil, nil
	})

	// fc_list_capabilities
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_list_capabilities",
		Description: "List available processors, supported file types, and external dependency status",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in listCapabilitiesInput) (*mcp.CallToolResult, any, error) {
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
		return textResult(string(data)), nil, nil
	})
}

func checkBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
