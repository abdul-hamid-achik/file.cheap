# fcheap E2E Tests

End-to-end tests for the fcheap CLI, built with [glyphrun](https://github.com/abdul-hamid-achik/glyphrun).

## Running

```bash
task e2e                                              # run all flows
glyph run e2e/flows/<name>.yml --format md            # run one flow
glyph context latest --format md                      # after a failure, get context
```

CI pins Glyphrun `v0.15.0` and runs `cli_artifact_ref.yml` as a release gate.
That focused flow builds the CLI, saves a real stash, emits Cairntrace and
Glyphrun ArtifactRefV1 envelopes, and verifies the missing-stash failure. The
same job runs the in-memory MCP tests and a separate `go install` smoke that
exercises the installed binary's save-to-ArtifactRef contract.
Glyphrun `v0.15.0` requires Go 1.26, so that isolated CI job uses Go 1.26.5;
the file.cheap module and release build remain pinned by `go.mod` to Go 1.25.12.

## Layout

```
e2e/
├── README.md           # this file
├── flows/              # one spec per user-facing flow
│   ├── cli_doctor.yml
│   ├── cli_save_list.yml
│   ├── cli_restore.yml
│   ├── cli_drop.yml
│   ├── cli_info.yml
│   ├── cli_compress.yml
│   ├── cli_analyze_search.yml
│   ├── cli_diff.yml
│   └── cli_config.yml
└── actions/           # reusable step snippets
    └── wait_clean_exit.yml
```

## Conventions

- Each spec has a `preconditions` block that builds the binary (`go build -o ./bin/fcheap ./cmd/fcheap`).
- Each spec uses `FCHEAP_STASH_DIR` env var to isolate stash storage per test.
- Fixtures are created inline in the `cmd` shell script for determinism.
- All specs import `wait_clean_exit` to wait for the process to finish.
- Outcomes verify both exit code (0) and screen content (contains key strings).
