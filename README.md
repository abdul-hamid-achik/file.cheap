# file.cheap

Local file processing CLI and MCP server for images, PDFs, and videos. Everything runs on your machine -- no cloud, no accounts, no uploads.

## Install

```bash
# macOS (Homebrew)
brew install abdul-hamid-achik/tap/fc

# Linux (deb)
curl -LO https://github.com/abdul-hamid-achik/file.cheap/releases/latest/download/fc_linux_amd64.deb
sudo dpkg -i fc_linux_amd64.deb

# From source
go install github.com/abdul-hamid-achik/file.cheap/cmd/fc@latest
```

Optional dependencies for full functionality:

```bash
# macOS
brew install ffmpeg poppler webp

# Ubuntu/Debian
sudo apt install ffmpeg poppler-utils libwebp-tools
```

Run `fc doctor` to check what's available.

## Usage

```bash
# Image operations
fc resize photo.jpg 800x600
fc thumbnail *.jpg
fc convert photo.png webp
fc optimize photo.jpg --quality 80
fc watermark photo.jpg "Copyright 2026"
fc info photo.jpg

# PDF
fc thumbnail document.pdf

# Video (requires ffmpeg)
fc thumbnail video.mp4
fc convert video.mov mp4

# Chain operations
fc process photo.jpg -t resize,optimize,webp

# Batch
fc resize images/*.jpg 1200x800 --parallel 8
```

## MCP Server

Use `fc` as an MCP tool server for AI assistants like Claude:

```json
{
  "mcpServers": {
    "file-cheap": {
      "command": "fc",
      "args": ["mcp", "serve"]
    }
  }
}
```

This exposes 14 tools: `fc_resize_image`, `fc_thumbnail`, `fc_convert_to_webp`, `fc_optimize_image`, `fc_convert_image`, `fc_watermark_image`, `fc_image_metadata`, `fc_pdf_thumbnail`, `fc_video_thumbnail`, `fc_transcode_video`, `fc_video_watermark`, `fc_generate_hls`, `fc_batch_process`, `fc_list_capabilities`.

## Configuration

```bash
fc config init       # create ~/.config/fc/config.yaml
fc config show       # print current config
fc config set quality 90
```

Config file (`~/.config/fc/config.yaml`):

```yaml
quality: 85
output_dir: ""       # empty = same directory as input
parallel: 8
overwrite: false
```

Environment variables: `FC_QUALITY`, `FC_OUTPUT_DIR`, `FC_JOBS`.

## Project Structure

```
file.cheap/
├── cmd/fc/                  # CLI entry point
├── internal/
│   ├── engine/              # Processing orchestration
│   ├── mcp/                 # MCP server (14 tools)
│   ├── processor/           # Core processing (zero deps on infra)
│   │   ├── image/           # 7 image processors
│   │   ├── pdf/             # PDF thumbnail
│   │   └── video/           # Video thumbnail, transcode, HLS, watermark
│   ├── fc/cli/              # Cobra commands
│   ├── fc/config/           # YAML config
│   ├── fc/output/           # Printer, progress bars, tables
│   ├── storage/             # Storage interface + local filesystem
│   ├── presets/             # Built-in processing presets
│   ├── apperror/            # Error types
│   └── logger/              # slog wrapper
└── testdata/                # Test fixtures
```

## Tech Stack

- **Go 1.25**, single static binary (~13MB), `CGO_ENABLED=0`
- **Image processing**: `disintegration/imaging`, `fogleman/gg` (pure Go)
- **Video**: ffmpeg/ffprobe (external, runtime-detected)
- **PDF**: poppler-utils or mupdf (external, runtime-detected)
- **MCP**: `modelcontextprotocol/go-sdk` (official SDK)
- **CLI**: `spf13/cobra`, `fatih/color`, `schollz/progressbar`

## License

MIT
