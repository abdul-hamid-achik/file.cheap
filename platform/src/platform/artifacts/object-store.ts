export type ArtifactObjectMetadata = {
  contentType: string;
  etag: string;
  key: string;
  sizeBytes: number;
  uploadedAt: string;
};

export type ArtifactTransferGrant = {
  expiresAt: string;
  headers: Record<string, string>;
  method: "GET" | "PUT";
  url: string;
};

export interface ArtifactObjectStore {
  readonly driver: string;
  delete(key: string, signal?: AbortSignal): Promise<void>;
  inspect(key: string, signal?: AbortSignal): Promise<ArtifactObjectMetadata | null>;
  /**
   * Recompute the object's SHA-256 by digesting its bytes incrementally and
   * report whether it equals `expectedSha256`. Implementations must never
   * buffer the whole object: memory stays O(1) regardless of `maxBytes`. A
   * missing object returns `false`; an object longer than `maxBytes` aborts the
   * transfer and throws.
   */
  verifySha256(key: string, expectedSha256: string, maxBytes: number, signal?: AbortSignal): Promise<boolean>;
  issueDownloadGrant(input: { key: string; validUntil: Date }, signal?: AbortSignal): Promise<ArtifactTransferGrant>;
  issueUploadGrant(input: { contentType: string; key: string; sizeBytes: number; validUntil: Date }, signal?: AbortSignal): Promise<ArtifactTransferGrant>;
}

const keyPattern = /^[a-z0-9][a-z0-9._/-]*$/;

export function assertSafeArtifactObjectKey(key: string): void {
  const parts = key.split("/");
  if (!keyPattern.test(key) || key.includes("..") || key.startsWith("/") || key.endsWith("/") || parts.some((part) => !part || part === "." || part === "..")) {
    throw new Error("Unsafe artifact object key");
  }
}
