# info

Get detailed information about a stash including the file tree and metadata.

## Usage

```bash
fcheap info <stash-id> [flags]
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output as JSON |

## Examples

```bash
fcheap info my_artifacts_20260622_115254
fcheap info my_artifacts_20260622_115254 --json
```

## Output

```
Stash Info

  ID: my_artifacts_20260622_115254
  Created: 2026-06-22T11:52:54Z
  Source: /tmp/artifacts
  Tool: vidtrace
  Bundle: vidtrace
  Video: /Downloads/EXAMPLE-1234.mp4 (124s, 30fps)
  Files: 805
  Size: 45.2 MB
  Content Hash: 52008249f8beedbfe99a598b346750524b0ec1c48987dca88320d73f4528f051
  Tags: [EXAMPLE-1234]
```

For vidtrace bundles, the `Video` line shows the source video and (when present)
its duration and frame rate, pulled from the bundle's `metadata.json` at save
time. A `! N potential secret(s)` warning appears when the save-time scan found
likely credentials.

```

Files (805)
  ├─ README.txt (1.2 KB)
  ├─ metadata.json (850 B)
  ├─ timeline.json (537 KB)
  ├─ frames/
  │   ├─ frame_0000.png (120 KB)
  │   ├─ frame_0001.png (118 KB)
  │   └─ ...
  ├─ ocr/
  │   ├─ frame_0000.txt (245 B)
  │   └─ ...
  └─ transcript/
      ├─ transcript.txt (12 KB)
      ├─ transcript.vtt (15 KB)
      ├─ transcript.srt (14 KB)
      └─ ...
```
