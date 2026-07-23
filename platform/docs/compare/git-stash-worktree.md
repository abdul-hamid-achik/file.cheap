# file.cheap vs Git stash and Git worktree

Git stash, Git worktree, and file.cheap all help you set work aside, but they
preserve different things. Git tools manage repository state. file.cheap
manages arbitrary files and folders produced around an agent workflow.

The practical answer is often to use both: keep source changes in Git and keep
screenshots, transcripts, reports, logs, and reproduction evidence in a
file.cheap stash.

## At a glance

| Need | Git stash | Git worktree | file.cheap |
|---|---|---|---|
| Set aside tracked source changes | Best fit | Commit them on a branch | Can snapshot them, but loses Git semantics |
| Work on two branches at once | No | Best fit | No |
| Preserve untracked binary evidence | Requires care and flags | Lives in one working tree | Best fit |
| Save any file or folder outside a repo | No | No | Yes |
| Record tool, source, tags, and TTL | No | Through separate conventions | Built in |
| Search file contents across snapshots | No cross-stash index | Normal repository tools | BM25, semantic, or hybrid |
| Verify restored files against a manifest | Git object integrity | Git object integrity | Per-file hash verification |
| Connect evidence text to likely source code | No | No | Optional vecgrep workflow |
| Compress old payloads | Git object database behavior | Shared Git object database | Explicit tar+zstd or gzip |

## Use Git stash for a short interruption

`git stash` is excellent when you need to switch context before a working-tree
change is ready to commit. It understands tracked changes and Git's index. It is
not designed to become a searchable catalog of external evidence, and a stash
message does not carry file.cheap's tool provenance, TTL, or manifest.

Use it when the sentence is: "Hold these code edits while I fix something
else."

## Use Git worktree for parallel branches

`git worktree` gives another branch its own checkout while sharing the same Git
repository. It is the right choice when two agents or two tasks need independent
working directories without repeatedly switching branches.

Use it when the sentence is: "Keep both branches checked out and runnable."

## Use file.cheap for durable workflow artifacts

file.cheap accepts any file or directory, whether it belongs in Git or not. A
stash can contain a vidtrace bundle, generated report, screenshots, logs, or a
temporary export while preserving the producing tool and original source.

```bash
fcheap save /tmp/login-repro \
  --tag login \
  --tag repro \
  --tool vidtrace \
  --source ~/Downloads/login-bug.mp4 \
  --index
```

Use it when the sentence is: "Preserve this evidence so an agent or engineer can
find, verify, and reopen it later."

## A combined workflow

1. Create a Git worktree for the repair branch.
2. Save the completed reproduction bundle with file.cheap.
3. Search or connect that stash to the worktree's code.
4. Commit source changes in Git.
5. Keep the evidence until review is complete, then assign a TTL or drop it.

```bash
git worktree add ../storefront-fix -b fix/login-refresh
fcheap connect <stash-id> ../storefront-fix --index
git -C ../storefront-fix status
```

Do not use a file vault as a substitute for meaningful commits and branches.
Do not put large, private investigation artifacts into Git merely because they
need a lifecycle. Pick the tool whose data model matches the thing you are
preserving.

See [`save`](/cli/save), [`diff`](/cli/diff), and
[`connect`](/cli/connect) for the file.cheap side of the combined workflow.
