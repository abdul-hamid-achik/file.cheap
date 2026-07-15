import { createHash, randomUUID } from "node:crypto";
import {
  createReadStream,
  createWriteStream,
  promises as filesystem,
} from "node:fs";
import { dirname, join } from "node:path";
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

export class LocalObjectStore implements ObjectStore {
  readonly driver = "local" as const;

  constructor(private readonly config: PlatformConfig = getConfig()) {}

  async inspect(key: string): Promise<ObjectMetadata | null> {
    const path = this.pathFor(key);
    try {
      const stat = await filesystem.stat(path);
      return {
        contentType: contentTypeFor(key),
        etag: await hashFile(path),
        key,
        sizeBytes: stat.size,
        uploadedAt: stat.mtime.toISOString(),
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
    const current = await this.inspect(input.key);
    if (
      (input.expectedEtag && current?.etag !== input.expectedEtag) ||
      (!input.expectedEtag && current)
    ) {
      throw new CatalogPreconditionError();
    }

    const path = this.pathFor(input.key);
    await filesystem.mkdir(dirname(path), { mode: 0o700, recursive: true });
    const temporaryPath = `${path}.${randomUUID()}.tmp`;
    await filesystem.writeFile(temporaryPath, input.body, {
      flag: "wx",
      mode: 0o600,
    });
    await syncFile(temporaryPath);
    await filesystem.rename(temporaryPath, path);
    return { etag: hashBytes(Buffer.from(input.body)) };
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
        source.destroy(new Error("Upload exceeded its signed size"));
      }
    });

    try {
      await pipeline(
        source,
        createWriteStream(temporaryPath, { flags: "wx", mode: 0o600 }),
      );
      const receivedHash = hash.digest("hex");
      if (receivedBytes !== payload.sizeBytes || receivedHash !== payload.sha256) {
        throw new PlatformError({
          code: "integrity_mismatch",
          detail: "Uploaded bytes do not match the signed size and SHA-256.",
          status: 422,
          title: "Integrity mismatch",
        });
      }

      await syncFile(temporaryPath);

      try {
        await filesystem.link(temporaryPath, path);
      } catch (error) {
        if (!isNodeError(error, "EEXIST")) {
          throw error;
        }
      }
    } finally {
      await filesystem.rm(temporaryPath, { force: true });
    }

    const metadata = await this.inspect(key);
    if (!metadata) {
      throw new Error("Upload completed without creating the object");
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
        etag: metadata.etag,
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
