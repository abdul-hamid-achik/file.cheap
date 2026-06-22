---
layout: home

hero:
  name: file.cheap
  text: Local-first stash tool
  tagline: Save, restore, compress, analyze, and diff files for agent workflows. No cloud, no accounts, no uploads.
  actions:
    - theme: brand
      text: Get Started
      link: /guide/getting-started
    - theme: alt
      text: View on GitHub
      link: https://github.com/abdul-hamid-achik/file.cheap

features:
  - title: Save & Restore
    details: Snapshot files and folders into a managed vault with provenance tracking. Restore on demand to any directory.
  - title: Compress
    details: tar+zstd archiving with automatic threshold-based compression. Save disk space without losing data.
  - title: Analyze
    details: Built-in BM25 keyword search via veclite. Optional semantic search with vecgrep subprocess integration.
  - title: Diff
    details: Compare any stash against a live codebase to see what changed, what's missing, and what matches.
  - title: MCP Server
    details: Expose stash operations as MCP tools for AI assistants like Claude. 8 tools with typed schemas.
  - title: Studio TUI
    details: Browse stashes, view manifests, and trigger operations from a terminal UI built with Bubbletea v2.
  - title: Bundle Detection
    details: Automatically detects vidtrace bundles and extracts searchable text (OCR + transcripts). Generic detector handles everything else.
  - title: Agent-Friendly
    details: Designed for agent workflows -- save vidtrace artifacts, analyze with vecgrep, diff against source code, drop when done.
---