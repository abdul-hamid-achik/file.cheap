# config

Manage fcheap configuration.

## Usage

```bash
fcheap config <subcommand> [flags]
```

## Subcommands

| Command | Description |
|---------|-------------|
| `fcheap config show` | Show current configuration |
| `fcheap config init` | Create default config file |
| `fcheap config set <key> <value>` | Set a config value |
| `fcheap config get <key>` | Get a config value |
| `fcheap config path` | Show config file path |

## Config Keys

| Key | Type | Description |
|-----|------|-------------|
| `stash_dir` | string | Stash storage directory |
| `compression` | string | Compression algorithm (zstd, gzip) |
| `compress_threshold` | int | Auto-compress threshold in bytes |
| `log_level` | string | Log level (debug, info, warn, error) |
| `vecgrep_path` | string | Path to vecgrep binary |
| `embedder` | string | Embedding provider (for semantic search) |
| `embed_model` | string | Embedding model name |
| `ollama_url` | string | Ollama base URL |

## Examples

```bash
# Show current config
fcheap config show

# Initialize default config
fcheap config init

# Set stash directory
fcheap config set stash_dir ~/.local/share/fcheap

# Set compression
fcheap config set compression zstd

# Get a value
fcheap config get stash_dir

# Show config file path
fcheap config path
```