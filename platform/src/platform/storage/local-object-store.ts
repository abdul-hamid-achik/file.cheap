import { createHash, randomUUID } from "node:crypto";
import {
  createReadStream,
  createWriteStream,
  promises as filesystem,
} from "node:fs";
import { basename, dirname, join } from "node:path";
import { Readable } from "node:stream";
import { pipeline } from "node:stream/promises";

import { getConfig, type PlatformConfig } from "@/shared/config/env";
import {
  CatalogPreconditionError,
  PlatformError,
} from "@/shared/errors/platform-error";
import {
  signPayload,
  verifyPayload,
} from "@/shared/security/signed-token";
import {
  assertSafeObjectKey,
  type ObjectMetadata,
  type ObjectStore,
  type TextObject,
  type TransferGrant,
} from "@/platform/storage/object-store";

const localCatalogLockDeadlineMilliseconds = 15_000;

export class LocalObjectStore implements ObjectStore {
  readonly driver = "local" as const;
  readonly verification = "server-sha256" as const;

  constructor(private readonly config: PlatformConfig = getConfig()) {}

  async inspect(key: string): Promise<ObjectMetadata | null> {
    const path = this.pathFor(key);
    try {
      const stat = await filesystem.stat(path);
      const sha256 = await hashFile(path);
      return {
        contentType: contentTypeFor(key),
        etag: sha256,
        key,
        sizeBytes: stat.size,
        uploadedAt: stat.mtime.toISOString(),
        verifiedSha256: sha256,
      };
    } catch (error) {
      if (isNodeError(error, "ENOENT")) {
        return null;
      }
      throw error;
    }
  }

  async issueUploadGrant(input: {
    contentType: string;
    key: string;
    sha256: string;
    sizeBytes: number;
    validUntil: Date;
  }): Promise<TransferGrant> {
    assertSafeObjectKey(input.key);
    const token = signPayload(
      {
        contentType: input.contentType,
        exp: input.validUntil.getTime(),
        key: input.key,
        kind: "upload",
        sha256: input.sha256,
        sizeBytes: input.sizeBytes,
      },
      this.config.signingSecret,
    );
    const query = new URLSearchParams({ key: input.key, token });

    return {
      expiresAt: input.validUntil.toISOString(),
      headers: { "content-type": input.contentType },
      method: "PUT",
      url: `${this.config.publicUrl}/api/v1/local-objects?${query}`,
    };
  }

  async issueDownloadGrant(input: {
    key: string;
    validUntil: Date;
  }): Promise<TransferGrant> {
    assertSafeObjectKey(input.key);
    const token = signPayload(
      {
        exp: input.validUntil.getTime(),
        key: input.key,
        kind: "download",
      },
      this.config.signingSecret,
    );
    const query = new URLSearchParams({ key: input.key, token });

    return {
      expiresAt: input.validUntil.toISOString(),
      headers: {},
      method: "GET",
      url: `${this.config.publicUrl}/api/v1/local-objects?${query}`,
    };
  }

  async readText(key: string): Promise<TextObject | null> {
    const metadata = await this.inspect(key);
    if (!metadata) {
      return null;
    }

    return {
      body: await filesystem.readFile(this.pathFor(key), "utf8"),
      etag: metadata.etag,
    };
  }

  async writeText(input: {
    body: string;
    expectedEtag?: string;
    key: string;
  }): Promise<{ etag: string }> {
    const path = this.pathFor(input.key);
    await filesystem.mkdir(dirname(path), { mode: 0o700, recursive: true });
    return withFileLock(`${path}.lock`, async (assertOwned) => {
      const current = await this.inspect(input.key);
      if (
        (input.expectedEtag && current?.etag !== input.expectedEtag) ||
        (!input.expectedEtag && current)
      ) {
        throw new CatalogPreconditionError();
      }

      const temporaryPath = `${path}.${randomUUID()}.tmp`;
      try {
        await filesystem.writeFile(temporaryPath, input.body, {
          flag: "wx",
          mode: 0o600,
        });
        await syncFile(temporaryPath);
        await assertOwned();
        await filesystem.rename(temporaryPath, path);
        await syncDirectory(dirname(path));
      } finally {
        await filesystem.rm(temporaryPath, { force: true });
      }
      return { etag: hashBytes(Buffer.from(input.body)) };
    });
  }

  async acceptUpload(request: Request, key: string, token: string): Promise<ObjectMetadata> {
    const payload = verifyPayload(token, this.config.signingSecret);
    if (payload.kind !== "upload" || payload.key !== key) {
      throw new PlatformError({
        code: "grant_scope_mismatch",
        detail: "The upload grant does not authorize this object.",
        status: 403,
        title: "Grant scope mismatch",
      });
    }

    if (request.headers.get("content-type") !== payload.contentType) {
      throw new PlatformError({
        code: "content_type_mismatch",
        detail: `The upload must use content type ${payload.contentType}.`,
        status: 400,
        title: "Content type mismatch",
      });
    }
    if (!request.body) {
      throw new PlatformError({
        code: "empty_upload",
        detail: "The upload body is required.",
        status: 400,
        title: "Empty upload",
      });
    }

    const existing = await this.inspect(key);
    if (existing) {
      if (existing.sizeBytes === payload.sizeBytes && existing.etag === payload.sha256) {
        return existing;
      }
      throw new PlatformError({
        code: "object_conflict",
        detail: "An immutable object already exists at this key.",
        status: 409,
        title: "Object conflict",
      });
    }

    const path = this.pathFor(key);
    await filesystem.mkdir(dirname(path), { mode: 0o700, recursive: true });
    const temporaryPath = `${path}.${randomUUID()}.tmp`;
    const hash = createHash("sha256");
    let receivedBytes = 0;
    const source = Readable.fromWeb(request.body as never);
    source.on("data", (chunk: Buffer) => {
      receivedBytes += chunk.length;
      hash.update(chunk);
      if (receivedBytes > payload.sizeBytes) {
        source.destroy(
          new PlatformError({
            code: "upload_too_large",
            detail: "The upload exceeded its signed byte size.",
            status: 413,
            title: "Upload too large",
          }),
        );
      }
    });

    try {
      await pipeline(
        source,
        createWriteStream(temporaryPath, { flags: "wx", mode: 0o600 }),
      );
      const receivedHash = hash.digest("hex");
      if (receivedBytes !== payload.sizeBytes || receivedHash !== payload.sha256) {
        throw uploadIntegrityMismatch();
      }

      await syncFile(temporaryPath);

      let created = false;
      try {
        await filesystem.link(temporaryPath, path);
        created = true;
      } catch (error) {
        if (!isNodeError(error, "EEXIST")) {
          throw error;
        }
      }
      if (created) await syncDirectory(dirname(path));
    } finally {
      await filesystem.rm(temporaryPath, { force: true });
    }

    const metadata = await this.inspect(key);
    if (!metadata) {
      throw new Error("Upload completed without creating the object");
    }
    if (
      metadata.sizeBytes !== payload.sizeBytes ||
      metadata.verifiedSha256 !== payload.sha256
    ) {
      throw uploadIntegrityMismatch();
    }
    return { ...metadata, contentType: payload.contentType };
  }

  async serveDownload(key: string, token: string): Promise<Response> {
    const payload = verifyPayload(token, this.config.signingSecret);
    if (payload.kind !== "download" || payload.key !== key) {
      throw new PlatformError({
        code: "grant_scope_mismatch",
        detail: "The download grant does not authorize this object.",
        status: 403,
        title: "Grant scope mismatch",
      });
    }

    const metadata = await this.inspect(key);
    if (!metadata) {
      throw new PlatformError({
        code: "object_not_found",
        detail: "The requested archive object does not exist.",
        status: 404,
        title: "Object not found",
      });
    }

    const stream = Readable.toWeb(
      createReadStream(this.pathFor(key)),
    ) as unknown as ReadableStream;
    return new Response(stream, {
      headers: {
        "content-length": String(metadata.sizeBytes),
        "content-type": metadata.contentType,
        etag: `"${metadata.etag}"`,
      },
    });
  }

  private pathFor(key: string): string {
    assertSafeObjectKey(key);
    return join(this.config.dataDirectory, "objects", ...key.split("/"));
  }
}

async function hashFile(path: string): Promise<string> {
  const hash = createHash("sha256");
  for await (const chunk of createReadStream(path)) {
    hash.update(chunk);
  }
  return hash.digest("hex");
}

function hashBytes(bytes: Buffer): string {
  return createHash("sha256").update(bytes).digest("hex");
}

function contentTypeFor(key: string): string {
  return key.endsWith(".json")
    ? "application/json"
    : "application/vnd.filecheap.stash";
}

function isNodeError(error: unknown, code: string): boolean {
  return error instanceof Error && "code" in error && error.code === code;
}

async function syncFile(path: string): Promise<void> {
  const file = await filesystem.open(path, "r");
  try {
    await file.sync();
  } finally {
    await file.close();
  }
}

async function syncDirectory(path: string): Promise<void> {
  const directory = await filesystem.open(path, "r");
  try {
    await directory.sync();
  } finally {
    await directory.close();
  }
}

async function withFileLock<T>(
  lockPath: string,
  operation: (assertOwned: () => Promise<void>) => Promise<T>,
): Promise<T> {
  const deadline = Date.now() + localCatalogLockDeadlineMilliseconds;
  let lock: OwnedFileLock | null = null;

  while (!lock) {
    lock = await tryAcquireFileLock(lockPath);
    if (lock) break;

    if (await recoverAbandonedLock(lockPath, deadline)) {
      continue;
    }

    if (Date.now() >= deadline) {
      throw catalogBusy();
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }

  const assertOwned = async (): Promise<void> => {
    let current: Awaited<ReturnType<typeof filesystem.stat>>;
    try {
      current = await filesystem.stat(lockPath);
    } catch (error) {
      if (isNodeError(error, "ENOENT")) throw catalogLockLost();
      throw error;
    }
    if (
      current.dev !== lock.identity.dev ||
      current.ino !== lock.identity.ino
    ) {
      throw catalogLockLost();
    }
  };
  try {
    return await operation(assertOwned);
  } finally {
    try {
      await lock.handle.close();
    } finally {
      await removeLockIfOwned(lockPath, lock.identity);
    }
  }
}

type LockIdentity = { dev: number; ino: number };

type LockOwner = {
  pid: number;
  token: string;
  version: 1;
};

type LockObservation = {
  identity: LockIdentity;
  owner: LockOwner | null;
};

type OwnedFileLock = LockObservation & {
  handle: Awaited<ReturnType<typeof filesystem.open>>;
  owner: LockOwner;
};

type RecoveryClaim = {
  path: string;
  pid: number;
  rank: string;
};

async function tryAcquireFileLock(lockPath: string): Promise<OwnedFileLock | null> {
  // Avoid making every waiter write and fsync a candidate while a canonical
  // lock is visibly present. The hard-link publication below remains the
  // correctness boundary if the lock disappears or appears after this hint.
  try {
    await filesystem.stat(lockPath);
    return null;
  } catch (error) {
    if (!isNodeError(error, "ENOENT")) throw error;
  }

  const owner: LockOwner = {
    pid: process.pid,
    token: randomUUID(),
    version: 1,
  };
  const candidatePath = `${lockPath}.candidate.${owner.token}`;
  const handle = await filesystem.open(candidatePath, "wx", 0o600);
  let acquired = false;

  try {
    // Publish a fully written owner record with one no-overwrite link. A
    // contender can therefore never observe a half-initialized canonical lock.
    await handle.writeFile(`${JSON.stringify(owner)}\n`);
    await handle.sync();
    try {
      await filesystem.link(candidatePath, lockPath);
    } catch (error) {
      if (isNodeError(error, "EEXIST")) return null;
      throw error;
    }

    const stat = await handle.stat();
    acquired = true;
    await filesystem.rm(candidatePath, { force: true });
    return {
      handle,
      identity: { dev: stat.dev, ino: stat.ino },
      owner,
    };
  } finally {
    if (!acquired) {
      await handle.close();
      await filesystem.rm(candidatePath, { force: true });
    }
  }
}

async function observeLock(lockPath: string): Promise<LockObservation | null> {
  let handle: Awaited<ReturnType<typeof filesystem.open>>;
  try {
    handle = await filesystem.open(lockPath, "r");
  } catch (error) {
    if (isNodeError(error, "ENOENT")) return null;
    throw error;
  }

  try {
    const [body, stat] = await Promise.all([
      handle.readFile("utf8"),
      handle.stat(),
    ]);
    return {
      identity: { dev: stat.dev, ino: stat.ino },
      owner: parseLockOwner(body),
    };
  } finally {
    await handle.close();
  }
}

async function recoverAbandonedLock(
  lockPath: string,
  deadline: number,
): Promise<boolean> {
  const observed = await observeLock(lockPath);
  if (!observed) return true;
  // Do not expire a live owner by elapsed time. Recovery is allowed only after
  // its process has ended, so a delayed writer cannot resume behind a new one.
  if (!observed.owner || isProcessAlive(observed.owner.pid)) return false;

  const claim = await createRecoveryClaim(lockPath);

  try {
    while (Date.now() < deadline) {
      const current = await observeLock(lockPath);
      if (!current) return true;
      if (!sameIdentity(current.identity, observed.identity)) return true;
      if (!current.owner || isProcessAlive(current.owner.pid)) return false;

      const claims = await listActiveRecoveryClaims(lockPath);
      if (claims[0]?.path !== claim.path) {
        await new Promise((resolve) => setTimeout(resolve, 10));
        continue;
      }

      const finalObservation = await observeLock(lockPath);
      if (!finalObservation) return true;
      if (!sameIdentity(finalObservation.identity, observed.identity)) return true;
      if (
        !finalObservation.owner ||
        isProcessAlive(finalObservation.owner.pid)
      ) {
        return false;
      }

      const quarantinePath = `${lockPath}.abandoned.${claim.rank}.${randomUUID()}`;
      await filesystem.rename(lockPath, quarantinePath);
      await filesystem.rm(quarantinePath, { force: true });
      await syncDirectory(dirname(lockPath));
      return true;
    }
    return false;
  } finally {
    await filesystem.rm(claim.path, { force: true });
  }
}

async function createRecoveryClaim(lockPath: string): Promise<RecoveryClaim> {
  // Unique ordered claims elect one recoverer without a shared marker that
  // another contender could accidentally replace during cleanup.
  const rank = process.hrtime.bigint().toString(16).padStart(16, "0");
  const path = `${lockPath}.recovery.${rank}-${process.pid}-${randomUUID()}`;
  await filesystem.writeFile(path, "", { flag: "wx", mode: 0o600 });
  return { path, pid: process.pid, rank };
}

async function listActiveRecoveryClaims(
  lockPath: string,
): Promise<RecoveryClaim[]> {
  const directory = dirname(lockPath);
  const prefix = `${basename(lockPath)}.recovery.`;
  const claims: RecoveryClaim[] = [];

  for (const entry of await filesystem.readdir(directory, { withFileTypes: true })) {
    if (!entry.isFile() || !entry.name.startsWith(prefix)) continue;
    const match = /^([0-9a-f]+)-(\d+)-/.exec(entry.name.slice(prefix.length));
    if (!match) continue;

    const claim = {
      path: join(directory, entry.name),
      pid: Number(match[2]),
      rank: match[1],
    };
    if (isProcessAlive(claim.pid)) {
      claims.push(claim);
    } else {
      await filesystem.rm(claim.path, { force: true });
    }
  }

  return claims.sort(
    (left, right) =>
      left.rank.localeCompare(right.rank) || left.path.localeCompare(right.path),
  );
}

function parseLockOwner(body: string): LockOwner | null {
  try {
    const owner = JSON.parse(body) as Partial<LockOwner>;
    if (
      owner.version !== 1 ||
      !Number.isInteger(owner.pid) ||
      Number(owner.pid) <= 0 ||
      typeof owner.token !== "string" ||
      owner.token.length === 0
    ) {
      return null;
    }
    return owner as LockOwner;
  } catch {
    return null;
  }
}

function isProcessAlive(pid: number): boolean {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    if (isNodeError(error, "ESRCH")) return false;
    if (isNodeError(error, "EPERM")) return true;
    throw error;
  }
}

function sameIdentity(left: LockIdentity, right: LockIdentity): boolean {
  return left.dev === right.dev && left.ino === right.ino;
}

async function removeLockIfOwned(
  lockPath: string,
  identity: { dev: number; ino: number },
): Promise<void> {
  let current: Awaited<ReturnType<typeof filesystem.stat>>;
  try {
    current = await filesystem.stat(lockPath);
  } catch (error) {
    if (isNodeError(error, "ENOENT")) return;
    throw error;
  }
  if (current.dev === identity.dev && current.ino === identity.ino) {
    await filesystem.rm(lockPath, { force: true });
  }
}

function catalogBusy(): PlatformError {
  return new PlatformError({
    code: "catalog_busy",
    detail: "The local catalog is busy. Retry the operation.",
    status: 503,
    title: "Catalog busy",
  });
}

function catalogLockLost(): PlatformError {
  return new PlatformError({
    code: "catalog_lock_lost",
    detail: "The local catalog lock was replaced. Retry the operation.",
    status: 503,
    title: "Catalog lock lost",
  });
}

function uploadIntegrityMismatch(): PlatformError {
  return new PlatformError({
    code: "integrity_mismatch",
    detail: "Uploaded bytes do not match the signed size and SHA-256.",
    status: 422,
    title: "Integrity mismatch",
  });
}
