# Local-first vs cloud artifact storage for AI agents

"Where should agent artifacts live?" sounds like a storage question, but the
right answer depends on the job. Fast local investigation, disaster recovery,
multi-device access, and team sharing have different requirements. Treating
them as one generic sync problem usually produces either a cluttered laptop or
a cloud bucket full of sensitive, unsearchable files.

file.cheap is local-first today: manifests, payloads, SQLite metadata, and the
veclite search index live on your machine. Remote embedding is optional and is
separate from storage. Understanding that boundary makes it easier to decide
when local storage is enough and when a separate cloud copy is justified.

## What local-first buys you

### Immediate access

Saving, listing, reading manifests, and restoring files do not wait for a
network. An agent can use the same workflow on a plane, behind a restrictive
corporate proxy, or during a provider outage.

### A smaller trust boundary

Bug evidence often includes logs, source fragments, screenshots, local paths,
and accidental credentials. Keeping the vault local avoids granting another
service routine access to that material. file.cheap's save-time scanner is a
warning system, not a proof that a bundle contains no secret, so reducing the
number of systems that receive the bytes remains valuable.

### Predictable performance and cost

Local reads do not incur request or egress charges. Large video-derived bundles
can be compressed once with zstd and reopened without downloading them again.

## What local-first does not solve

Local-only storage is not a backup. A failed or stolen disk can remove both the
original artifact and its stash. It also does not automatically make the same
stash available on a second workstation or to a teammate.

Those are valid reasons to add a remote copy, but they do not require turning
the cloud into the primary source of truth. A hybrid design can keep the
manifest and recently used content local while treating remote object storage as
a verified recovery tier.

## When cloud storage helps

Cloud storage is useful when at least one of these is true:

- The artifact would be expensive or impossible to reproduce.
- You need to continue the investigation from another device.
- A teammate needs the exact evidence, hashes, and provenance.
- Local SSD capacity is the constraint and on-demand rehydration is acceptable.
- A retention policy requires a copy independent of one workstation.

Cloud storage is less compelling for compiler output, generated code maps, or
other caches that are cheaper to regenerate than upload, secure, catalog, and
download.

## Compare the operating models

| Concern | Local-first vault | Generic cloud bucket | Hybrid vault + remote copy |
|---|---|---|---|
| Offline save and list | Yes | No | Yes |
| Second-device recovery | Manual | Yes | Yes |
| Searchable stash metadata | Built in | Must be designed | Local, with optional remote catalog |
| Secret exposure | One machine by default | Provider and credentials enter the boundary | Can use client-side encryption |
| Disk reclamation | Compression and deletion | Local files can be removed | Verified upload, then explicit eviction |
| Restore integrity | Manifest hashes | Depends on client | Verify remote bytes and manifest hashes |
| Collaboration | Export or restore manually | Access policy required | Share immutable stash references |

## The safe hybrid sequence

A cloud copy saves local disk only when the workflow explicitly evicts local
payload bytes. The safe sequence is:

1. Compress a complete stash.
2. Encrypt it locally if the remote operator should not see plaintext.
3. Upload to an immutable object name.
4. Verify the remote size and cryptographic digest.
5. Commit the remote catalog record.
6. Keep the manifest locally.
7. Evict the local payload only after the user requests it.
8. Download to a temporary path and verify before any later restore.

Calling this "sync" can hide the most important distinction: removing a local
cache is not the same operation as deleting the remote recovery copy. A reliable
product should expose separate commands for offloading, hydrating, and deleting
everywhere.

## A practical policy

Start with three classes:

- **Hot evidence:** keep locally and compress above the configured threshold.
- **Recoverable evidence:** keep a verified remote copy; allow explicit local
  eviction after testing a restore.
- **Regenerable cache:** assign a TTL and let [`fcheap sweep`](/cli/sweep) remove
  it instead of paying to archive it indefinitely.

This policy preserves the benefit of a local-first tool while leaving room for
backup and collaboration. If you are comparing concrete tools, see
[file.cheap vs cloud artifact storage](/compare/cloud-artifact-storage). For the
current local workflow, start with [`save`](/cli/save),
[`compress`](/cli/compress), and [`restore`](/cli/restore).
