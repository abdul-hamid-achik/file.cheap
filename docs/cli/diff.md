# diff

Compare a saved stash with a corresponding current directory. The comparison
uses relative paths and content hashes to show what was added, removed, or
changed after the snapshot.

## Usage

```bash
fcheap diff <stash-id> <target-dir> [flags]
```

## Arguments

| Argument | Description |
|---|---|
| `stash-id` | Opaque ID of the saved baseline |
| `target-dir` | Current directory tree to compare with that baseline |

The target directory must exist. Global `--json` output is available for
automation.

## Choose a meaningful target

`diff` compares file trees; it does not search by meaning. Use it when the stash
and target represent versions of the same artifact, generated output, export,
or working directory.

Good examples include:

- a generated report before and after changing its generator;
- exported configuration before and after an upgrade;
- a fixture directory before and after a test run;
- a source tree snapshot compared with the same source tree later.

Do not diff an OCR, screenshot, or vidtrace bundle against an application
repository. Those trees contain unrelated paths. Use [`connect`](/cli/connect)
to rank related code candidates from evidence text.

## Example

Create a baseline:

```bash
fcheap save ./generated-site --name "Generated site baseline"
```

After regenerating the same directory, compare it with the saved ID:

```bash
fcheap diff <stash-id> ./generated-site
```

## How it works

1. Reads the manifest and payload from the stash, extracting an archive when
   necessary.
2. Walks the saved and target directory trees.
3. Compares normalized relative paths.
4. Hashes files present in both trees and compares their content.
5. Reports files only in the stash, only in the target, changed, and unchanged.

The command does not modify the stash or target.

## Output

```text
Diff: generated-site_20260715_120000 vs /home/user/project/generated-site

Only in stash (1 file):
  removed-page.html

Only in target (1 file):
  new-page.html

Changed (2 files):
  index.html
  assets/app.css

Unchanged: 24 files
```

The exact paths and counts depend on the compared trees. With `--json`, use the
file arrays and unchanged count rather than parsing the human display.

## See also

- [`save`](/cli/save) — create the baseline
- [`restore`](/cli/restore) — materialize and verify the complete saved tree
- [`connect`](/cli/connect) — relate evidence text to likely source code
