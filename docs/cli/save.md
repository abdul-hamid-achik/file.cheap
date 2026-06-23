# save

Save a file or directory to the stash vault.

## Usage

```bash
fcheap save <path> [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | derived from path | Display name for the stash |
| `--tag` | string slice | `[]` | Tags for categorization (repeatable) |
| `--tool` | string | `""` | Tool that produced the content (e.g., vidtrace) |
| `--source` | string | `""` | Original artifact this stash derives from (provenance) |
| `--no-scan` | bool | `false` | Skip the save-time secret scan |

## Examples

```bash
# Save a directory
fcheap save /tmp/artifacts --tag bug-123 --tool vidtrace

# Save with multiple tags
fcheap save ./report.pdf --tag evidence --tag pdf --tool manual

# Save with source provenance
fcheap save /tmp/vidtrace-output --tag OPG-15061 --tool vidtrace --source ~/Downloads/OPG-15061.mp4

# Save a single file
fcheap save ./config.yaml --tag config
```

## What Happens

1. fcheap resolves the path to an absolute path
2. Creates a stash directory at `<stash-dir>/<stash-id>/`
3. Copies the file tree into `content/`
4. Generates a `manifest.json` with metadata, provenance, file count, size, and content hashes
5. Auto-detects bundle type (vidtrace, generic)
6. Scans content for likely secrets (unless `--no-scan`) and records findings in the manifest
7. Prints the stash ID and summary

## Secret scanning

On save, fcheap scans text files for likely credentials — AWS/GitHub/Slack/Google
keys, private keys, JWTs, and generic `key = secret` assignments. It records only
the **file, rule, and line** (never the secret value) in the manifest and prints a
warning so you don't archive live credentials into a shareable stash. Use
`--no-scan` to skip, or review with [`info`](/cli/info). A `⚠ secrets` chip also
appears in [Studio](/studio/overview).

## Output

```
Saved stash: my_artifacts_20260622_115254
  Source: /tmp/artifacts
  Tool: vidtrace
  Bundle: vidtrace
  Files: 805
  Size: 45.2 MB
  Tags: [OPG-15061]
! 2 potential secret(s) detected in this stash — review before sharing or restoring elsewhere
  └─ .env:1 [aws-access-key]
  └─ config.yaml:7 [generic-secret]
```