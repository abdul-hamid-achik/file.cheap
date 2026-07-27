# pull

Download one committed artifact from the private console and verify its exact
bytes before making it available at a new local path.

## Usage

```bash
fcheap auth login
fcheap pull <artifact-id> --output <new-file>
```

The paired device must own the artifact. `pull` refreshes an expired short-lived
access credential when the stored refresh family is still valid, requests a
one-minute direct download grant, and then downloads from private object storage
without proxying the bytes through the file.cheap application.

## Flags

| Flag | Type | Required | Description |
|---|---|---:|---|
| `--output`, `-o` | path | yes | New path for the verified artifact bytes |
| `--json` | bool | no | Emit the stable `filecheap-pull/1` result |

The parent directory must already exist. The output path must not exist; the
command never overwrites files.

```bash
fcheap pull art_abcdefghijklmnop --output ./glyphrun-run.tar.zst
```

## Integrity and transfer boundary

The console response binds the artifact ID, recorded size, SHA-256, content
type, and a credential-free `ArtifactRefV1`. Large bytes use the short-lived
signed transfer directly. `pull`:

1. rejects redirects and unsafe grant headers;
2. streams at most the recorded size into a mode-`0600` staging file;
3. verifies the exact byte count and SHA-256;
4. atomically installs the verified file at the requested path.

The signed URL is never printed, included in JSON, or written to the credential
file. A failed or mismatched download leaves no destination file.

## Safe inspection

`pull` restores the immutable uploaded object. It deliberately does not extract,
render, execute, or preview the contents. Treat archives, HTML, videos, logs,
and documents as untrusted input and inspect them with an appropriate sandbox or
read-only tool.

To recover a local stash, use [`restore`](/cli/restore) instead. `pull` is only
for artifacts that were explicitly published to the private service.

## JSON result

```json
{
  "version": "filecheap-pull/1",
  "artifact_ref": {
    "$schema": "urn:filecheap.dev:artifact-ref:v1",
    "version": 1,
    "provider": "fcheap-cloud",
    "uri": "fcheap://cloud/vaults/private/artifacts/art_abcdefghijklmnop",
    "artifact_id": "art_abcdefghijklmnop",
    "kind": "glyphrun.run"
  },
  "output_path": "/absolute/path/glyphrun-run.tar.zst",
  "sha256": "…",
  "size_bytes": 1234,
  "verification": "server-sha256"
}
```

The JSON result contains no access token, refresh token, signed URL, object key,
or storage credential.
