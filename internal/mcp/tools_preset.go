package mcp

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/presets"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// -- Input structs for preset tools --

type applyPresetInput struct {
	Path       string `json:"path" jsonschema:"Absolute path to the image file"`
	Preset     string `json:"preset" jsonschema:"Preset name: thumbnail, sm, md, lg, xl, og, twitter, instagram_square, instagram_portrait, or instagram_story"`
	OutputPath string `json:"output_path,omitempty" jsonschema:"Output file path (auto-generated if omitted)"`
}

type listPresetsInput struct{}

func registerPresetTools(srv *mcp.Server, eng *engine.Engine) {
	falseVal := false

	// fc_apply_preset
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_apply_preset",
		Description: "Apply a named preset to resize or crop an image. Presets include social media sizes (og, twitter, instagram), responsive breakpoints (sm, md, lg, xl), and thumbnails.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in applyPresetInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}

		p, ok := presets.Get(in.Preset)
		if !ok {
			r, _ := toolError("unknown preset: %s", in.Preset)
			return r, nil, nil
		}

		var procName string
		opts := &processor.Options{
			Width:   p.Width,
			Height:  p.Height,
			Quality: p.Quality,
		}

		if p.Crop {
			procName = "thumbnail"
			opts.Position = "center"
		} else {
			procName = "resize"
		}

		return processAndRespond(ctx, eng, &engine.Request{
			InputPath:  absPath(in.Path),
			OutputPath: in.OutputPath,
			Processor:  procName,
			Options:    opts,
		})
	})

	// fc_list_presets
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_list_presets",
		Description: "List all available presets with their dimensions, quality, and category",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in listPresetsInput) (*mcp.CallToolResult, any, error) {
		type presetInfo struct {
			Name     string `json:"name"`
			Width    int    `json:"width"`
			Height   int    `json:"height"`
			Quality  int    `json:"quality"`
			Crop     bool   `json:"crop"`
			Category string `json:"category"`
		}

		var items []presetInfo
		for name, p := range presets.All {
			cat := "general"
			if presets.IsSocialPreset(name) {
				cat = "social"
			} else if presets.IsResponsivePreset(name) {
				cat = "responsive"
			} else if strings.HasPrefix(name, "pdf_") {
				cat = "pdf"
			}
			items = append(items, presetInfo{
				Name:     name,
				Width:    p.Width,
				Height:   p.Height,
				Quality:  p.Quality,
				Crop:     p.Crop,
				Category: cat,
			})
		}

		sort.Slice(items, func(i, j int) bool {
			if items[i].Category != items[j].Category {
				return items[i].Category < items[j].Category
			}
			return items[i].Name < items[j].Name
		})

		data, _ := json.MarshalIndent(items, "", "  ")
		return textResult(string(data)), nil, nil
	})
}
