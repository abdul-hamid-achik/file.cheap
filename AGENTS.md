# AGENTS.md

Guidelines for AI agents working on the file.cheap codebase.

## Architecture

file.cheap is a local-first CLI tool + MCP server. There is no database, no API server, no cloud infrastructure. Everything processes files locally on the user's machine.

### Key Layers

1. **Processors** (`internal/processor/`) -- the core. Each processor implements `Processor` interface: `Process(ctx, *Options, io.Reader) (*Result, error)`. Zero coupling to infrastructure. Never import packages outside `processor/`.

2. **Engine** (`internal/engine/`) -- orchestration. Opens files, picks the right processor from the registry, writes output, returns metadata. The bridge between CLI/MCP and processors.

3. **MCP Server** (`internal/mcp/`) -- exposes 14 tools via `modelcontextprotocol/go-sdk`. Uses typed input structs with `json` + `jsonschema` tags for auto-schema generation. Each tool validates input, calls engine, returns JSON.

4. **CLI** (`internal/fc/cli/`) -- Cobra commands. Each command file handles args/flags, calls `engine.Process()`, prints output via the printer.

5. **Storage** (`internal/storage/`) -- interface with local filesystem implementation. Used by engine for file I/O.

## Code Style

- Go 1.25+, `CGO_ENABLED=0`
- No generics unless the stdlib pattern demands it
- Errors: wrap with `fmt.Errorf("context: %w", err)`, use `apperror` types at boundaries
- Logging: `internal/logger` (slog-based). Use `slog.Debug`/`slog.Info`/`slog.Error`
- Tests: `testing` + `testify/assert`. Test files next to source
- Lint: `golangci-lint` with `errcheck` enabled. All deferred `.Close()` calls on read-only files use `//nolint:errcheck` comment

## Conventions

### Adding a New Processor

1. Create `internal/processor/<type>/<name>.go` implementing `processor.Processor`
2. Register in `internal/engine/engine.go` `RegisterDefaults()`
3. Add CLI command in `internal/fc/cli/<name>.go`
4. Add MCP tool in `internal/mcp/tools_<type>.go`
5. Add test with fixture in `testdata/`

### MCP Tools

Tools use the official Go SDK pattern:

```go
type myInput struct {
    Path    string `json:"path" jsonschema:"description=...,required"`
    Quality int    `json:"quality,omitempty" jsonschema:"description=...,minimum=1,maximum=100"`
}

mcp.AddTool(srv, &mcp.Tool{
    Name:        "fc_my_tool",
    Description: "...",
}, func(ctx context.Context, req *mcp.CallToolRequest, in myInput) (*mcp.CallToolResult, any, error) {
    // validate, process, return
})
```

### CLI Commands

Each command follows the pattern:

```go
var myCmd = &cobra.Command{
    Use:   "my <files...>",
    Short: "...",
    Args:  cobra.MinimumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        // iterate files, call eng.Process(), print results
    },
}
```

Register in `root.go` `init()`.

## External Dependencies

- **ffmpeg/ffprobe** -- required for all video operations. Check with `engine.FFmpegAvailable()`
- **pdftoppm/pdfinfo** (poppler-utils) or **mutool** (mupdf) -- required for PDF thumbnails
- **cwebp** -- optional for WebP, pure Go fallback exists

Never bundle these. Detect at runtime, show clear errors with `fc doctor` instructions.

## What NOT to Do

- Don't add database dependencies
- Don't add HTTP server/API code
- Don't add authentication or billing
- Don't import cloud SDKs (S3, GCS, etc.)
- Don't add telemetry, metrics, or tracing
- Don't bundle ffmpeg or other binaries
- Don't use `os/exec` outside of `processor/video/` and `processor/pdf/`
