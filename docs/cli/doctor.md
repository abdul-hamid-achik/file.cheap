# doctor

Check runtime health and dependency status.

## Usage

```bash
fcheap doctor
```

## What It Checks

- **Stash directory**: exists and writable
- **Config file**: loaded and valid
- **SQLite**: embedded database accessible
- **veclite**: keyword search database accessible
- **vecgrep**: optional semantic search binary (reports if found in PATH or configured path)
- **Compression**: zstd library available (built-in)

## Output

```
fcheap doctor: ok

  Stash dir: ~/.local/share/fcheap (writable)
  Config: ~/.config/fcheap/config.yaml (loaded)
  SQLite: ok
  veclite: ok
  vecgrep: not found (optional, for semantic search)
  zstd: built-in (klauspost/compress)
```

If there are issues, doctor reports them with suggestions:

```
fcheap doctor: issues found

  Stash dir: /custom/path (not found)
  Config: ~/.config/fcheap/config.yaml (loaded)
  SQLite: ok
  vecgrep: found at /usr/local/bin/vecgrep
  zstd: built-in

Run `fcheap config set stash_dir ~/.local/share/fcheap` to fix.
```