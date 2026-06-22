# diff

Compare a stash against a target directory. Shows files only in stash, only in target, and changed files.

## Usage

```bash
fcheap diff <stash-id> <target-dir> [flags]
```

## Arguments

| Argument | Description |
|----------|-------------|
| `stash-id` | The stash ID to compare |
| `target-dir` | Target directory to compare against |

## Examples

```bash
# Compare a stash against a live codebase
fcheap diff my_artifacts_20260622_115254 ~/projects/graphite

# Compare against a specific subdirectory
fcheap diff my_artifacts_20260622_115254 ~/projects/graphite/go/service/warehouse
```

## How It Works

1. Extracts the stash content (from tree or archive)
2. Walks both directories and compares file paths
3. For files in both, compares content hashes
4. Reports: only-in-stash, only-in-target, changed, unchanged

## Output

```
Diff: my_artifacts_20260622_115254 vs ~/projects/graphite

Only in stash (12 files):
  frames/frame_0000.png
  frames/frame_0001.png
  ...

Only in target (45 files):
  go/service/warehouse/main.go
  go/service/warehouse/handler.go
  ...

Changed (3 files):
  go/service/warehouse/conditions.go
  go/service/warehouse/types.go
  go/service/warehouse/render.go

Unchanged: 8 files
```