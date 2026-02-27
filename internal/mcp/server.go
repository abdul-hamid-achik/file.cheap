package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/file.cheap/internal/engine"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewServer creates a configured MCP server with all file.cheap tools registered.
// The caller runs it via server.Run(ctx, &mcp.StdioTransport{}).
func NewServer(eng *engine.Engine, version string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "file.cheap",
		Title:   "file.cheap - Local File Processing",
		Version: version,
	}, &mcp.ServerOptions{
		Instructions: "file.cheap provides local file processing tools for images, PDFs, and videos. " +
			"All processing happens locally on your machine. " +
			"Use fcheap_list_capabilities to check available processors and dependencies. " +
			"Use fcheap_file_info to inspect files before processing. " +
			"Use fcheap_list_presets for quick named presets (social media sizes, responsive breakpoints). " +
			"Chain operations with fcheap_pipeline. Video tools require ffmpeg (check with fcheap_list_capabilities).",
	})

	// Tools
	registerImageTools(srv, eng)
	registerPDFTools(srv, eng)
	registerVideoTools(srv, eng)
	registerUtilTools(srv, eng)
	registerInfoTools(srv, eng)
	registerPresetTools(srv, eng)
	registerPipelineTools(srv, eng)

	// Resources & Prompts
	registerResources(srv, eng)
	registerPrompts(srv)

	return srv
}

// resultSummary marshals a processing result into a JSON summary string.
func resultSummary(res *engine.Result) string {
	summary := map[string]any{
		"input_path":  res.InputPath,
		"output_path": res.OutputPath,
		"input_size":  res.InputSize,
		"output_size": res.OutputSize,
		"duration_ms": res.Duration.Milliseconds(),
	}
	if res.Width > 0 {
		summary["width"] = res.Width
	}
	if res.Height > 0 {
		summary["height"] = res.Height
	}
	if res.Format != "" {
		summary["format"] = res.Format
	}
	b, _ := json.MarshalIndent(summary, "", "  ")
	return string(b)
}

// toolError returns a CallToolResult that signals a tool-level error to the LLM.
func toolError(msg string, args ...any) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(msg, args...)}},
		IsError: true,
	}, nil
}

// textResult returns a successful CallToolResult with text content.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

// validatePath checks that a file path exists and is accessible.
func validatePath(path string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path %q: %w", path, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("file not found: %s", abs)
	}
	return nil
}

// absPath returns the absolute path for a given path string.
func absPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// processAndRespond runs a single engine request and returns the MCP result.
func processAndRespond(ctx context.Context, eng *engine.Engine, req *engine.Request) (*mcp.CallToolResult, any, error) {
	res, err := eng.Process(ctx, req)
	if err != nil {
		r, _ := toolError("processing failed: %v", err)
		return r, nil, nil
	}
	return textResult(resultSummary(res)), nil, nil
}
