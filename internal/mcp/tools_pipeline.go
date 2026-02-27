package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type pipelineStep struct {
	Operation string `json:"operation" jsonschema:"Processor name: resize, thumbnail, webp, optimize, convert, or watermark"`
	Width     int    `json:"width,omitempty" jsonschema:"Width in pixels"`
	Height    int    `json:"height,omitempty" jsonschema:"Height in pixels"`
	Quality   int    `json:"quality,omitempty" jsonschema:"Output quality 1-100"`
	Format    string `json:"format,omitempty" jsonschema:"Target format for convert operation"`
	Text      string `json:"text,omitempty" jsonschema:"Text for watermark operation"`
	Fit       string `json:"fit,omitempty" jsonschema:"Fit mode for resize: contain, cover, or fill"`
	Position  string `json:"position,omitempty" jsonschema:"Position for thumbnail or watermark"`
}

type pipelineInput struct {
	Path       string         `json:"path" jsonschema:"Absolute path to the source file"`
	Steps      []pipelineStep `json:"steps" jsonschema:"Ordered list of processing steps"`
	OutputPath string         `json:"output_path,omitempty" jsonschema:"Final output file path (auto-generated if omitted)"`
}

var validPipelineOps = map[string]bool{
	"resize":    true,
	"thumbnail": true,
	"webp":      true,
	"optimize":  true,
	"convert":   true,
	"watermark": true,
}

func registerPipelineTools(srv *mcp.Server, eng *engine.Engine) {
	falseVal := false

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "fc_pipeline",
		Description: "Chain multiple processing operations on a single file. Each step's output becomes the next step's input. Useful for workflows like: resize \u2192 optimize \u2192 convert to webp.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseVal,
			OpenWorldHint:   &falseVal,
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in pipelineInput) (*mcp.CallToolResult, any, error) {
		if err := validatePath(in.Path); err != nil {
			r, _ := toolError("%v", err)
			return r, nil, nil
		}

		if len(in.Steps) == 0 {
			r, _ := toolError("at least one step is required")
			return r, nil, nil
		}
		if len(in.Steps) > 10 {
			r, _ := toolError("maximum 10 steps allowed, got %d", len(in.Steps))
			return r, nil, nil
		}

		for i, s := range in.Steps {
			if !validPipelineOps[s.Operation] {
				r, _ := toolError("step %d: unknown operation %q (valid: resize, thumbnail, webp, optimize, convert, watermark)", i+1, s.Operation)
				return r, nil, nil
			}
		}

		// Track intermediate temp files for cleanup.
		var tempFiles []string
		defer func() {
			for _, f := range tempFiles {
				os.Remove(f) //nolint:errcheck // best-effort cleanup
			}
		}()

		type stepResult struct {
			Operation  string `json:"operation"`
			DurationMs int64  `json:"duration_ms"`
		}

		currentInput := absPath(in.Path)
		totalStart := time.Now()
		stepResults := make([]stepResult, 0, len(in.Steps))
		var firstInputSize int64
		var lastResult *engine.Result

		for i, s := range in.Steps {
			select {
			case <-ctx.Done():
				r, _ := toolError("pipeline cancelled at step %d: %v", i+1, ctx.Err())
				return r, nil, nil
			default:
			}

			isLast := i == len(in.Steps)-1

			opts := &processor.Options{
				Width:   s.Width,
				Height:  s.Height,
				Quality: s.Quality,
				Format:  s.Format,
				Fit:     s.Fit,
			}
			if s.Position != "" {
				opts.Position = s.Position
			}
			if s.Text != "" {
				opts.VariantType = s.Text
			}

			engineReq := &engine.Request{
				InputPath: currentInput,
				Processor: s.Operation,
				Options:   opts,
			}

			if isLast && in.OutputPath != "" {
				engineReq.OutputPath = in.OutputPath
			} else if !isLast {
				// Build a temp file path for intermediate output.
				ext := filepath.Ext(currentInput)
				if s.Operation == "webp" {
					ext = ".webp"
				} else if s.Operation == "convert" && s.Format != "" {
					ext = "." + s.Format
				}
				tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("fc_pipeline_%d_%s%s", i, s.Operation, ext))
				engineReq.OutputPath = tmpPath
				tempFiles = append(tempFiles, tmpPath)
			}

			res, err := eng.Process(ctx, engineReq)
			if err != nil {
				r, _ := toolError("step %d (%s) failed: %v", i+1, s.Operation, err)
				return r, nil, nil
			}

			if i == 0 {
				firstInputSize = res.InputSize
			}

			stepResults = append(stepResults, stepResult{
				Operation:  s.Operation,
				DurationMs: res.Duration.Milliseconds(),
			})

			lastResult = res
			currentInput = res.OutputPath
		}

		totalDuration := time.Since(totalStart)

		out := map[string]any{
			"input_path":        absPath(in.Path),
			"output_path":       lastResult.OutputPath,
			"steps_completed":   len(in.Steps),
			"steps":             stepResults,
			"total_duration_ms": totalDuration.Milliseconds(),
			"input_size":        firstInputSize,
			"output_size":       lastResult.OutputSize,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		return textResult(string(data)), nil, nil
	})
}
