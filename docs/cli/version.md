# version

Print the fcheap version, commit, and build date.

## Usage

```bash
fcheap version
fcheap version --json
fcheap --version    # short form (version string only)
```

## Output

```
fcheap v0.16.0 (cca6e74) 2026-06-23T00:00:00Z
```

With `--json`:

```json
{
  "version": "v0.16.0",
  "commit": "cca6e74",
  "date": "2026-06-23T00:00:00Z"
}
```

The version, commit, and date are injected at build time via `-ldflags` (see the
`Taskfile.yml` `build` task). A source build without those flags reports
`dev (none) unknown`.
