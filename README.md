# file.cheap

Local file processing CLI and MCP server for images, PDFs, and videos. Everything runs on your machine -- no cloud, no accounts, no uploads.

## Install

```bash
# macOS (Homebrew)
brew install abdul-hamid-achik/tap/fcheap

# Linux (deb)
curl -LO https://github.com/abdul-hamid-achik/file.cheap/releases/latest/download/fcheap_linux_amd64.deb
sudo dpkg -i fcheap_linux_amd64.deb

# From source
go install github.com/abdul-hamid-achik/file.cheap/cmd/fcheap@latest
```

Optional dependencies for full functionality:

```bash
# macOS
brew install ffmpeg poppler webp

# Ubuntu/Debian
sudo apt install ffmpeg poppler-utils libwebp-tools
```

Run `fcheap doctor` to check what's available.

## Usage

```bash
# Image operations
fcheap resize photo.jpg 800x600
fcheap thumbnail *.jpg
fcheap convert photo.png webp
fcheap optimize photo.jpg --quality 80
fcheap watermark photo.jpg "Copyright 2026"
fcheap info photo.jpg

# PDF
fcheap thumbnail document.pdf

# Video (requires ffmpeg)
fcheap thumbnail video.mp4
fcheap convert video.mov mp4

# Chain operations
fcheap process photo.jpg -t resize,optimize,webp

# Batch
fcheap resize images/*.jpg 1200x800 --parallel 8
```

## MCP Server

Use `fcheap` as an MCP tool server for AI assistants like Claude:

```json
{
  "mcpServers": {
    "file-cheap": {
      "command": "fcheap",
      "args": ["mcp", "serve"]
    }
  }
}
```

This exposes 14 tools: `fcheap_resize_image`, `fcheap_thumbnail`, `fcheap_convert_to_webp`, `fcheap_optimize_image`, `fcheap_convert_image`, `fcheap_watermark_image`, `fcheap_image_metadata`, `fcheap_pdf_thumbnail`, `fcheap_video_thumbnail`, `fcheap_transcode_video`, `fcheap_video_watermark`, `fcheap_generate_hls`, `fcheap_batch_process`, `fcheap_list_capabilities`.

## Configuration

```bash
fcheap config init       # create ~/.config/fcheap/config.yaml
fcheap config show       # print current config
fcheap config set quality 90
```

Config file (`~/.config/fcheap/config.yaml`):

```yaml
quality: 85
output_dir: ""       # empty = same directory as input
parallel: 8
overwrite: false
```

Environment variables: `FCHEAP_QUALITY`, `FCHEAP_OUTPUT_DIR`, `FCHEAP_JOBS`.

## Project Structure

```
file.cheap/
├── cmd/fcheap/              # CLI entry point
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
