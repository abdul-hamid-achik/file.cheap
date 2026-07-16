import { createHash, randomUUID } from "node:crypto";
import {
  createWriteStream,
  constants as filesystemConstants,
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
  throwIfStorageOperationAborted,
  type ObjectMetadata,
  type ObjectStore,
  type TextObject,
  type TransferGrant,
} from "@/platform/storage/object-store";

const localCatalogLockDeadlineMilliseconds = 15_000;
const localUploadDeadlineMilliseconds = 5 * 60_000;
const maximumLockRecordBytes = 4 * 1024;
const maximumProcessId = 2_147_483_647;
export const localTransferTokenHeader = "x-filecheap-transfer-token";

export type LocalObjectStoreOptions = {
  testHooks?: {
    afterOpen?: (input: {
      key: string;
      operation: "download" | "readText";
      path: string;
    }) => Promise<void> | void;
  };
  uploadDeadlineMilliseconds?: number;
};

/**
 * Local prototype adapter. Its crash-lock ownership checks require one host,
 * one PID namespace, and a local filesystem; never share dataDirectory across
 * machines or containers.
 */
export class LocalObjectStore implements ObjectStore {
  readonly driver = "local" as const;
  readonly verification = "server-sha256" as const;

  private readonly uploadDeadlineMilliseconds: number;
  private readonly testHooks: LocalObjectStoreOptions["testHooks"];

  constructor(
    private readonly config: PlatformConfig = getConfig(),
    options: LocalObjectStoreOptions = {},
  ) {
    this.uploadDeadlineMilliseconds =
      options.uploadDeadlineMilliseconds ?? localUploadDeadlineMilliseconds;
    this.testHooks = options.testHooks;
    if (
      !Number.isFinite(this.uploadDeadlineMilliseconds) ||
      this.uploadDeadlineMilliseconds <= 0
    ) {
      throw new Error("Local upload deadline must be a positive duration");
    }
  }

  async inspect(
    key: string,
    signal?: AbortSignal,
  ): Promise<ObjectMetadata | null> {
    throwIfStorageOperationAborted(signal);
    const path = this.pathFor(key);
    const handle = await openExistingFile(path);
    if (!handle) return null;
    try {
      const stat = await handle.stat();
      const sha256 = await hashFileHandle(handle, signal);
      throwIfStorageOperationAborted(signal);
      return {
        contentType: contentTypeFor(key),
        etag: sha256,
        key,
        sizeBytes: stat.size,
        uploadedAt: stat.mtime.toISOString(),
        verifiedSha256: sha256,
      };
    } finally {
      await handle.close();
    }
  }

  async issueUploadGrant(
    input: {
      contentType: string;
      key: string;
      sha256: string;
      sizeBytes: number;
      validUntil: Date;
    },
    signal?: AbortSignal,
  ): Promise<TransferGrant> {
    throwIfStorageOperationAborted(signal);
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
    const query = new URLSearchParams({ key: input.key });

    return {
      expiresAt: input.validUntil.toISOString(),
      headers: {
        "content-type": input.contentType,
        [localTransferTokenHeader]: token,
      },
      method: "PUT",
      url: `${this.config.publicUrl}/api/v1/local-objects?${query}`,
    };
  }

  async issueDownloadGrant(
    input: {
      key: string;
      validUntil: Date;
    },
    signal?: AbortSignal,
  ): Promise<TransferGrant> {
    throwIfStorageOperationAborted(signal);
    assertSafeObjectKey(input.key);
    const token = signPayload(
      {
        exp: input.validUntil.getTime(),
        key: input.key,
        kind: "download",
      },
      this.config.signingSecret,
    );
    const query = new URLSearchParams({ key: input.key });

    return {
      expiresAt: input.validUntil.toISOString(),
      headers: { [localTransferTokenHeader]: token },
      method: "GET",
      url: `${this.config.publicUrl}/api/v1/local-objects?${query}`,
    };
  }

  async readText(
    key: string,
    signal?: AbortSignal,
  ): Promise<TextObject | null> {
    throwIfStorageOperationAborted(signal);
    const path = this.pathFor(key);
    const handle = await openExistingFile(path);
    if (!handle) return null;
    try {
      await this.testHooks?.afterOpen?.({ key, operation: "readText", path });
      const body = await handle.readFile();
      throwIfStorageOperationAborted(signal);
      return {
        body: body.toString("utf8"),
        etag: hashBytes(body),
      };
    } finally {
      await handle.close();
    }
  }

  async writeText(
    input: {
      body: string;
      expectedEtag?: string;
      key: string;
    },
    signal?: AbortSignal,
  ): Promise<{ etag: string }> {
    throwIfStorageOperationAborted(signal);
    const path = this.pathFor(input.key);
    await filesystem.mkdir(dirname(path), { mode: 0o700, recursive: true });
    return withFileLock(
      `${path}.lock`,
      async (assertOwned) => {
        throwIfStorageOperationAborted(signal);
        const current = await this.inspect(input.key, signal);
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
          throwIfStorageOperationAborted(signal);
          await assertOwned();
          await filesystem.rename(temporaryPath, path);
          await syncDirectory(dirname(path));
          throwIfStorageOperationAborted(signal);
        } finally {
          await filesystem.rm(temporaryPath, { force: true });
        }
        return { etag: hashBytes(Buffer.from(input.body)) };
      },
      signal,
    );
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
    const advertisedLength = request.headers.get("content-length");
    if (
      advertisedLength !== null &&
      Number.isFinite(Number(advertisedLength)) &&
      Number(advertisedLength) > payload.sizeBytes
    ) {
      throw uploadTooLarge();
    }

    const existing = await this.inspect(key, request.signal);
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
    if (request.signal.aborted) {
      throw uploadCanceled();
    }

    const path = this.pathFor(key);
    await filesystem.mkdir(dirname(path), { mode: 0o700, recursive: true });
    const temporaryPath = `${path}.${randomUUID()}.tmp`;
    const hash = createHash("sha256");
    let receivedBytes = 0;
    const source = Readable.fromWeb(request.body as never);
    const uploadDeadline = Math.min(
      payload.exp,
      Date.now() + this.uploadDeadlineMilliseconds,
    );
    const deadlineError =
      uploadDeadline === payload.exp ? expiredUploadGrant : uploadTimedOut;
    const deadlineTimer = setTimeout(() => {
      source.destroy(deadlineError());
    }, Math.max(0, uploadDeadline - Date.now()));
    deadlineTimer.unref();
    const abortUpload = (): void => {
      source.destroy(uploadCanceled());
    };
    request.signal.addEventListener("abort", abortUpload, { once: true });
    if (request.signal.aborted) {
      abortUpload();
    }
    const assertTransferActive = (): void => {
      if (request.signal.aborted) throw uploadCanceled();
      if (Date.now() >= uploadDeadline) throw deadlineError();
    };
    source.on("data", (chunk: Buffer) => {
      receivedBytes += chunk.length;
      hash.update(chunk);
      if (receivedBytes > payload.sizeBytes) {
        source.destroy(uploadTooLarge());
      }
    });

    try {
      try {
        assertTransferActive();
        await pipeline(
          source,
          createWriteStream(temporaryPath, { flags: "wx", mode: 0o600 }),
        );
      } catch (error) {
        if (request.signal.aborted && !(error instanceof PlatformError)) {
          throw uploadCanceled();
        }
        throw error;
      }
      assertTransferActive();
      const receivedHash = hash.digest("hex");
      if (receivedBytes !== payload.sizeBytes || receivedHash !== payload.sha256) {
        throw uploadIntegrityMismatch();
      }

      await syncFile(temporaryPath);
      assertTransferActive();

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
      clearTimeout(deadlineTimer);
      request.signal.removeEventListener("abort", abortUpload);
      await filesystem.rm(temporaryPath, { force: true });
    }

    const metadata = await this.inspect(key, request.signal);
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

  async serveDownload(
    key: string,
    token: string,
    signal?: AbortSignal,
  ): Promise<Response> {
    throwIfStorageOperationAborted(signal);
    const payload = verifyPayload(token, this.config.signingSecret);
    if (payload.kind !== "download" || payload.key !== key) {
      throw new PlatformError({
        code: "grant_scope_mismatch",
        detail: "The download grant does not authorize this object.",
        status: 403,
        title: "Grant scope mismatch",
      });
    }

    const path = this.pathFor(key);
    const handle = await openExistingFile(path);
    if (!handle) {
      throw new PlatformError({
        code: "object_not_found",
        detail: "The requested archive object does not exist.",
        status: 404,
        title: "Object not found",
      });
    }

    let handedOff = false;
    try {
      await this.testHooks?.afterOpen?.({ key, operation: "download", path });
      const stat = await handle.stat();
      const etag = await hashFileHandle(handle, signal);
      throwIfStorageOperationAborted(signal);
      const response = new Response(fileHandleStream(handle, signal), {
        headers: {
          "content-length": String(stat.size),
          "content-type": contentTypeFor(key),
          etag: `"${etag}"`,
        },
      });
      handedOff = true;
      return response;
    } finally {
      if (!handedOff) await handle.close();
    }
  }

  private pathFor(key: string): string {
    assertSafeObjectKey(key);
    return join(this.config.dataDirectory, "objects", ...key.split("/"));
  }
}

type FileHandle = Awaited<ReturnType<typeof filesystem.open>>;

async function openExistingFile(path: string): Promise<FileHandle | null> {
  try {
    return await filesystem.open(path, "r");
  } catch (error) {
    if (isNodeError(error, "ENOENT")) return null;
    throw error;
  }
}

async function hashFileHandle(
  handle: FileHandle,
  signal?: AbortSignal,
): Promise<string> {
  const hash = createHash("sha256");
  const buffer = Buffer.allocUnsafe(64 * 1024);
  let position = 0;
  while (true) {
    throwIfStorageOperationAborted(signal);
    const { bytesRead } = await handle.read(
      buffer,
      0,
      buffer.byteLength,
      position,
    );
    if (bytesRead === 0) break;
    hash.update(buffer.subarray(0, bytesRead));
    position += bytesRead;
  }
  return hash.digest("hex");
}

function fileHandleStream(
  handle: FileHandle,
  signal?: AbortSignal,
): ReadableStream<Uint8Array> {
  let closed = false;
  let position = 0;
  const close = async (): Promise<void> => {
    if (closed) return;
    closed = true;
    await handle.close();
  };
  let controller: ReadableStreamDefaultController<Uint8Array> | undefined;
  const abort = (): void => {
    controller?.error(
      new PlatformError({
        code: "request_aborted",
        detail: "The download request was canceled.",
        status: 408,
        title: "Request canceled",
      }),
    );
    void close().catch(() => undefined);
  };
  return new ReadableStream<Uint8Array>({
    async cancel() {
      signal?.removeEventListener("abort", abort);
      await close();
    },
    async pull(controller) {
      try {
        const buffer = Buffer.allocUnsafe(64 * 1024);
        const { bytesRead } = await handle.read(
          buffer,
          0,
          buffer.byteLength,
          position,
        );
        if (bytesRead === 0) {
          controller.close();
          signal?.removeEventListener("abort", abort);
          await close();
          return;
        }
        position += bytesRead;
        controller.enqueue(buffer.subarray(0, bytesRead));
      } catch (error) {
        controller.error(error);
        signal?.removeEventListener("abort", abort);
        await close().catch(() => undefined);
      }
    },
    start(nextController) {
      controller = nextController;
      signal?.addEventListener("abort", abort, { once: true });
      if (signal?.aborted) abort();
    },
  });
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

async function abortableDelay(
  milliseconds: number,
  signal?: AbortSignal,
): Promise<void> {
  throwIfStorageOperationAborted(signal);
  await new Promise<void>((resolve, reject) => {
    const cleanup = (): void => {
      clearTimeout(timeout);
      signal?.removeEventListener("abort", abort);
    };
    const abort = (): void => {
      cleanup();
      try {
        throwIfStorageOperationAborted(signal);
      } catch (error) {
        reject(error);
      }
    };
    const timeout = setTimeout(() => {
      cleanup();
      resolve();
    }, milliseconds);
    signal?.addEventListener("abort", abort, { once: true });
    if (signal?.aborted) abort();
  });
}

async function withFileLock<T>(
  lockPath: string,
  operation: (assertOwned: () => Promise<void>) => Promise<T>,
  signal?: AbortSignal,
): Promise<T> {
  const deadline = Date.now() + localCatalogLockDeadlineMilliseconds;
  let lock: OwnedFileLock | null = null;

  while (!lock) {
    throwIfStorageOperationAborted(signal);
    if (Date.now() >= deadline) throw catalogBusy();
    lock = await tryAcquireFileLock(lockPath);
    if (lock) break;

    if (await recoverAbandonedLock(lockPath, deadline, signal)) {
      continue;
    }

    if (Date.now() >= deadline) {
      throw catalogBusy();
    }
    await abortableDelay(10, signal);
  }

  const assertOwned = async (): Promise<void> => {
    let current: Awaited<ReturnType<typeof filesystem.lstat>>;
    try {
      current = await filesystem.lstat(lockPath);
    } catch (error) {
      if (isNodeError(error, "ENOENT")) throw catalogLockLost();
      throw error;
    }
    if (
      !current.isFile() ||
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
      try {
        await removeLockIfOwned(lockPath, lock.identity);
      } finally {
        await filesystem.rm(lock.candidatePath, { force: true });
      }
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
  candidatePath: string;
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
    await filesystem.lstat(lockPath);
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
    const stat = await handle.stat();
    try {
      await filesystem.link(candidatePath, lockPath);
    } catch (error) {
      if (isNodeError(error, "EEXIST")) return null;
      throw error;
    }

    acquired = true;
    return {
      candidatePath,
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
  for (let attempt = 0; attempt < 8; attempt += 1) {
    let directoryEntry: Awaited<ReturnType<typeof filesystem.lstat>>;
    try {
      directoryEntry = await filesystem.lstat(lockPath);
    } catch (error) {
      if (isNodeError(error, "ENOENT")) return null;
      throw error;
    }
    if (!directoryEntry.isFile() || directoryEntry.size > maximumLockRecordBytes) {
      throw catalogLockInvalid();
    }

    let handle: Awaited<ReturnType<typeof filesystem.open>>;
    try {
      handle = await filesystem.open(
        lockPath,
        filesystemConstants.O_RDONLY |
          filesystemConstants.O_NOFOLLOW |
          filesystemConstants.O_NONBLOCK,
      );
    } catch (error) {
      if (isNodeError(error, "ENOENT")) return null;
      if (isNodeError(error, "ELOOP")) throw catalogLockInvalid();
      throw error;
    }

    try {
      const stat = await handle.stat();
      if (!stat.isFile() || stat.size > maximumLockRecordBytes) {
        throw catalogLockInvalid();
      }
      if (!sameIdentity(stat, directoryEntry)) continue;
      const body = await readBoundedLockRecord(handle);
      let latestEntry: Awaited<ReturnType<typeof filesystem.lstat>>;
      try {
        latestEntry = await filesystem.lstat(lockPath);
      } catch (error) {
        if (isNodeError(error, "ENOENT")) return null;
        throw error;
      }
      if (!latestEntry.isFile()) throw catalogLockInvalid();
      if (!sameIdentity(stat, latestEntry)) continue;
      return {
        identity: { dev: stat.dev, ino: stat.ino },
        owner: parseLockOwner(body),
      };
    } finally {
      await handle.close();
    }
  }
  return null;
}

async function recoverAbandonedLock(
  lockPath: string,
  deadline: number,
  signal?: AbortSignal,
): Promise<boolean> {
  throwIfStorageOperationAborted(signal);
  const observed = await observeLock(lockPath);
  if (!observed) return true;
  if (!observed.owner) throw catalogLockInvalid();
  // Do not expire a live owner by elapsed time. Recovery is allowed only after
  // its process has ended, so a delayed writer cannot resume behind a new one.
  if (isProcessAlive(observed.owner.pid)) return false;

  const claim = await createRecoveryClaim(lockPath);

  try {
    while (Date.now() < deadline) {
      throwIfStorageOperationAborted(signal);
      const current = await observeLock(lockPath);
      if (!current) return true;
      if (!sameIdentity(current.identity, observed.identity)) return true;
      if (!current.owner) throw catalogLockInvalid();
      if (isProcessAlive(current.owner.pid)) return false;

      const claims = await listActiveRecoveryClaims(lockPath);
      if (claims[0]?.path !== claim.path) {
        await abortableDelay(10, signal);
        continue;
      }

      throwIfStorageOperationAborted(signal);
      const finalObservation = await observeLock(lockPath);
      if (!finalObservation) return true;
      if (!sameIdentity(finalObservation.identity, observed.identity)) return true;
      if (!finalObservation.owner) throw catalogLockInvalid();
      if (isProcessAlive(finalObservation.owner.pid)) return false;

      const quarantinePath = `${lockPath}.abandoned.${claim.rank}.${randomUUID()}`;
      await filesystem.rename(lockPath, quarantinePath);
      await removeCandidateIfOwned(
        lockPath,
        finalObservation.owner,
        observed.identity,
      );
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
    const match = /^([0-9a-f]{16,32})-(\d+)-([0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12})$/.exec(
      entry.name.slice(prefix.length),
    );
    if (!match) continue;

    const pid = Number(match[2]);
    if (!Number.isInteger(pid) || pid <= 0 || pid > maximumProcessId) continue;
    const claim = {
      path: join(directory, entry.name),
      pid,
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
      Number(owner.pid) > maximumProcessId ||
      typeof owner.token !== "string" ||
      !/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(
        owner.token,
      )
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
  let current: Awaited<ReturnType<typeof filesystem.lstat>>;
  try {
    current = await filesystem.lstat(lockPath);
  } catch (error) {
    if (isNodeError(error, "ENOENT")) return;
    throw error;
  }
  if (
    current.isFile() &&
    current.dev === identity.dev &&
    current.ino === identity.ino
  ) {
    await filesystem.rm(lockPath, { force: true });
  }
}

async function readBoundedLockRecord(handle: FileHandle): Promise<string> {
  const buffer = Buffer.allocUnsafe(maximumLockRecordBytes + 1);
  let receivedBytes = 0;
  while (receivedBytes < buffer.byteLength) {
    const { bytesRead } = await handle.read(
      buffer,
      receivedBytes,
      buffer.byteLength - receivedBytes,
      receivedBytes,
    );
    if (bytesRead === 0) break;
    receivedBytes += bytesRead;
  }
  if (receivedBytes > maximumLockRecordBytes) throw catalogLockInvalid();
  return buffer.subarray(0, receivedBytes).toString("utf8");
}

async function removeCandidateIfOwned(
  lockPath: string,
  owner: LockOwner,
  identity: LockIdentity,
): Promise<void> {
  const candidatePath = `${lockPath}.candidate.${owner.token}`;
  let candidate: Awaited<ReturnType<typeof filesystem.lstat>>;
  try {
    candidate = await filesystem.lstat(candidatePath);
  } catch (error) {
    if (isNodeError(error, "ENOENT")) return;
    throw error;
  }
  if (candidate.isFile() && sameIdentity(candidate, identity)) {
    await filesystem.rm(candidatePath, { force: true });
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

function catalogLockInvalid(): PlatformError {
  return new PlatformError({
    code: "catalog_lock_invalid",
    detail: "The local catalog lock is malformed and requires inspection.",
    status: 503,
    title: "Invalid catalog lock",
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

function uploadTooLarge(): PlatformError {
  return new PlatformError({
    code: "upload_too_large",
    detail: "The upload exceeded its signed byte size.",
    status: 413,
    title: "Upload too large",
  });
}

function uploadCanceled(): PlatformError {
  return new PlatformError({
    code: "upload_canceled",
    detail: "The upload was canceled before it completed.",
    status: 408,
    title: "Upload canceled",
  });
}

function expiredUploadGrant(): PlatformError {
  return new PlatformError({
    code: "expired_grant",
    detail: "The upload grant expired before the transfer completed.",
    status: 410,
    title: "Expired grant",
  });
}

function uploadTimedOut(): PlatformError {
  return new PlatformError({
    code: "upload_timeout",
    detail: "The upload exceeded the server transfer deadline.",
    status: 408,
    title: "Upload timed out",
  });
}
