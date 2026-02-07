package mcp

import (
	"context"
	"encoding/json"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// -- Input structs for typed tool handlers --

type resizeInput struct {
	Path       string `json:"path" jsonschema:"description=Absolute path to the image file,required"`
	Width      int    `json:"width,omitempty" jsonschema:"description=Target width in pixels"`
	Height     int    `json:"height,omitempty" jsonschema:"description=Target height in pixels"`
	Quality    int    `json:"quality,omitempty" jsonschema:"description=Output quality 1-100,minimum=1,maximum=100"`
	Fit        string `json:"fit,omitempty" jsonschema:"description=Fit mode: contain cover or fill,enum=contain,enum=cover,enum=fill"`
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=Output file path (auto-generated if omitted)"`
}

type thumbnailInput struct {
	Path       string `json:"path" jsonschema:"description=Absolute path to the image file,required"`
	Width      int    `json:"width,omitempty" jsonschema:"description=Thumbnail width in pixels,default=300"`
	Height     int    `json:"height,omitempty" jsonschema:"description=Thumbnail height in pixels,default=300"`
	Position   string `json:"position,omitempty" jsonschema:"description=Crop anchor position,enum=center,enum=north,enum=south,enum=east,enum=west"`
	Quality    int    `json:"quality,omitempty" jsonschema:"description=Output quality 1-100,minimum=1,maximum=100"`
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=Output file path (auto-generated if omitted)"`
}

type webpInput struct {
	Path       string `json:"path" jsonschema:"description=Absolute path to the image file,required"`
	Quality    int    `json:"quality,omitempty" jsonschema:"description=Output quality 1-100,minimum=1,maximum=100"`
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=Output file path (auto-generated if omitted)"`
}

type optimizeInput struct {
	Path       string `json:"path" jsonschema:"description=Absolute path to the image file,required"`
	Quality    int    `json:"quality,omitempty" jsonschema:"description=Output quality 1-100,minimum=1,maximum=100"`
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=Output file path (auto-generated if omitted)"`
}

type convertImageInput struct {
	Path       string `json:"path" jsonschema:"description=Absolute path to the image file,required"`
	Format     string `json:"format" jsonschema:"description=Target format,required,enum=jpeg,enum=png,enum=gif,enum=bmp,enum=tiff"`
	Quality    int    `json:"quality,omitempty" jsonschema:"description=Output quality 1-100,minimum=1,maximum=100"`
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=Output file path (auto-generated if omitted)"`
}

type watermarkImageInput struct {
	Path       string `json:"path" jsonschema:"description=Absolute path to the image file,required"`
	Text       string `json:"text" jsonschema:"description=Watermark text to overlay,required"`
	Position   string `json:"position,omitempty" jsonschema:"description=Watermark position,enum=center,enum=bottom-right,enum=bottom-left,enum=top-right,enum=top-left"`
	Opacity    int    `json:"opacity,omitempty" jsonschema:"description=Watermark opacity 1-100,minimum=1,maximum=100"`
	FontSize   int    `json:"font_size,omitempty" jsonschema:"description=Font size in pixels"`
	OutputPath string `json:"output_path,omitempty" jsonschema:"description=Output file path (auto-generated if omitted)"`
}

type metadataInput struct {
	Path string `json:"path" jsonschema:"description=Absolute path to the image file,required"`
}

func registerImageTools(srv *mcp.Server, eng *engine.Engine) {
	falseVal := false

	// fc_resize_image
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_resize_image",
		Description: "Resize an image to the given dimensions",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in resizeInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		return processAndRespond(ctx, eng, &engine.Request{
			InputPath:  absPath(in.Path),
			OutputPath: in.OutputPath,
			Processor:  "resize",
			Options: &processor.Options{
				Width:   in.Width,
				Height:  in.Height,
				Quality: in.Quality,
				Fit:     in.Fit,
			},
		})
	})

	// fc_thumbnail
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_thumbnail",
		Description: "Generate a thumbnail from an image",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in thumbnailInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		w, h := in.Width, in.Height
		if w == 0 {
			w = 300
		}
		if h == 0 {
			h = 300
		}
		pos := in.Position
		if pos == "" {
			pos = "center"
		}
		return processAndRespond(ctx, eng, &engine.Request{
			InputPath:  absPath(in.Path),
			OutputPath: in.OutputPath,
			Processor:  "thumbnail",
			Options: &processor.Options{
				Width:    w,
				Height:   h,
				Position: pos,
				Quality:  in.Quality,
			},
		})
	})

	// fc_convert_to_webp
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_convert_to_webp",
		Description: "Convert an image to WebP format for smaller file sizes",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in webpInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		return processAndRespond(ctx, eng, &engine.Request{
			InputPath:  absPath(in.Path),
			OutputPath: in.OutputPath,
			Processor:  "webp",
			Options:    &processor.Options{Quality: in.Quality},
		})
	})

	// fc_optimize_image
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_optimize_image",
		Description: "Optimize an image to reduce file size while maintaining quality",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in optimizeInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		return processAndRespond(ctx, eng, &engine.Request{
			InputPath:  absPath(in.Path),
			OutputPath: in.OutputPath,
			Processor:  "optimize",
			Options:    &processor.Options{Quality: in.Quality},
		})
	})

	// fc_convert_image
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_convert_image",
		Description: "Convert an image to a different format (jpeg, png, gif, bmp, tiff)",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in convertImageInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		return processAndRespond(ctx, eng, &engine.Request{
			InputPath:  absPath(in.Path),
			OutputPath: in.OutputPath,
			Processor:  "convert",
			Options: &processor.Options{
				Format:  in.Format,
				Quality: in.Quality,
			},
		})
	})

	// fc_watermark_image
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_watermark_image",
		Description: "Add a text watermark to an image",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in watermarkImageInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		opacity := in.Opacity
		if opacity == 0 {
			opacity = 50
		}
		fontSize := in.FontSize
		if fontSize == 0 {
			fontSize = 24
		}
		pos := in.Position
		if pos == "" {
			pos = "bottom-right"
		}
		return processAndRespond(ctx, eng, &engine.Request{
			InputPath:  absPath(in.Path),
			OutputPath: in.OutputPath,
			Processor:  "watermark",
			Options: &processor.Options{
				VariantType: in.Text,
				Fit:         pos,
				Quality:     opacity,
				Width:       fontSize,
			},
		})
	})

	// fc_image_metadata
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_image_metadata",
		Description: "Read metadata (dimensions, format, size) from an image file. Returns JSON.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in metadataInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}
		res, err := eng.Process(ctx, &engine.Request{
			InputPath: absPath(in.Path),
			Processor: "metadata",
		})
		if err != nil {
			r, _ := toolError("metadata read failed: %v", err)
			return r, nil, nil
		}
		meta, _ := json.MarshalIndent(res.Metadata, "", "  ")
		return textResult(string(meta)), nil, nil
	})
}
