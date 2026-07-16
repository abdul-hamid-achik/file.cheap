import { PlatformError } from "@/shared/errors/platform-error";

export type ObjectMetadata = {
  contentType: string;
  etag: string;
  key: string;
  sizeBytes: number;
  uploadedAt: string;
  /** Present only when the adapter has hashed the complete stored object. */
  verifiedSha256?: string;
};

export type TransferGrant = {
  expiresAt: string;
  headers: Record<string, string>;
  method: "GET" | "PUT";
  url: string;
};

export type TextObject = {
  body: string;
  etag: string;
};

export interface ObjectStore {
  readonly driver: "local" | "vercel-blob";
  /** What the adapter can prove before a catalog commit. */
  readonly verification: "presence-size-etag" | "server-sha256";

  inspect(key: string, signal?: AbortSignal): Promise<ObjectMetadata | null>;

  issueUploadGrant(
    input: {
      contentType: string;
      key: string;
      sha256: string;
      sizeBytes: number;
      validUntil: Date;
    },
    signal?: AbortSignal,
  ): Promise<TransferGrant>;

  issueDownloadGrant(
    input: {
      key: string;
      validUntil: Date;
    },
    signal?: AbortSignal,
  ): Promise<TransferGrant>;

  readText(key: string, signal?: AbortSignal): Promise<TextObject | null>;

  writeText(
    input: {
      body: string;
      expectedEtag?: string;
      key: string;
    },
    signal?: AbortSignal,
  ): Promise<{ etag: string }>;
}

const objectKeyPattern = /^[a-z0-9][a-z0-9._/-]*$/;

export function assertSafeObjectKey(key: string): void {
  const segments = key.split("/");
  if (
    !objectKeyPattern.test(key) ||
    key.includes("..") ||
    key.startsWith("/") ||
    key.endsWith("/") ||
    segments.some((segment) => !segment || segment === "." || segment === "..")
  ) {
    throw new Error(`Unsafe object key: ${key}`);
  }
}

export function throwIfStorageOperationAborted(signal?: AbortSignal): void {
  if (!signal?.aborted) return;
  throw new PlatformError({
    code: "request_aborted",
    detail: "The request was canceled before the storage operation completed.",
    status: 408,
    title: "Request canceled",
  });
}
