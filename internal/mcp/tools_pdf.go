package mcp

import (
	"context"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type pdfThumbnailInput struct {
	Path       string `json:"path" jsonschema:"Absolute path to the PDF file"`
	Page       int    `json:"page,omitempty" jsonschema:"Page number to thumbnail, 1-based (default 1)"`
	Width      int    `json:"width,omitempty" jsonschema:"Thumbnail width in pixels"`
	Height     int    `json:"height,omitempty" jsonschema:"Thumbnail height in pixels"`
	Format     string `json:"format,omitempty" jsonschema:"Output image format: png or jpeg"`
	OutputPath string `json:"output_path,omitempty" jsonschema:"Output file path (auto-generated if omitted)"`
}

func registerPDFTools(srv *mcp.Server, eng *engine.Engine) {
	falseVal := false

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fcheap_pdf_thumbnail",
		Description: "Generate a thumbnail image from a PDF page. Requires poppler-utils or mutool.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in pdfThumbnailInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		page := in.Page
		if page == 0 {
			page = 1
		}
		format := in.Format
		if format == "" {
			format = "png"
		}
		return processAndRespond(ctx, eng, &engine.Request{
			InputPath:  absPath(in.Path),
			OutputPath: in.OutputPath,
			Processor:  "pdf_thumbnail",
			Options: &processor.Options{
				Page:   page,
				Width:  in.Width,
				Height: in.Height,
				Format: format,
			},
		})
	})
}
