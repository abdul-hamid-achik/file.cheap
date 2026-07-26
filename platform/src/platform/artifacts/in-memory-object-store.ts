import { createHash, randomUUID } from "node:crypto";

import { assertSafeArtifactObjectKey, type ArtifactObjectMetadata, type ArtifactObjectStore, type ArtifactTransferGrant } from "@/platform/artifacts/object-store";

const verificationChunkBytes = 64 * 1024;

type Stored = ArtifactObjectMetadata & { bytes?: Uint8Array };

/** Test adapter only. Route tests seed completed direct uploads explicitly. */
export class InMemoryArtifactObjectStore implements ArtifactObjectStore {
  readonly driver = "memory";
  private readonly objects = new Map<string, Stored>();

  async delete(key: string): Promise<void> { this.objects.delete(key); }
  async inspect(key: string): Promise<ArtifactObjectMetadata | null> { return this.objects.get(key) ?? null; }
  async verifySha256(key: string, expectedSha256: string, maxBytes: number): Promise<boolean> {
    const bytes = this.objects.get(key)?.bytes;
    if (!bytes) return false;
    if (bytes.byteLength > maxBytes) {
      throw new Error("Private artifact exceeds the verification limit");
    }
    const digest = createHash("sha256");
    for (let offset = 0; offset < bytes.byteLength; offset += verificationChunkBytes) {
      digest.update(bytes.subarray(offset, offset + verificationChunkBytes));
    }
    return digest.digest("hex") === expectedSha256;
  }
  async issueDownloadGrant(input: { key: string; validUntil: Date }): Promise<ArtifactTransferGrant> {
    assertSafeArtifactObjectKey(input.key);
    return { expiresAt: input.validUntil.toISOString(), headers: {}, method: "GET", url: `https://artifact.test/download/${input.key}` };
  }
  async issueUploadGrant(input: { contentType: string; key: string; sizeBytes: number; validUntil: Date }): Promise<ArtifactTransferGrant> {
    assertSafeArtifactObjectKey(input.key);
    return { expiresAt: input.validUntil.toISOString(), headers: { "content-type": input.contentType }, method: "PUT", url: `https://artifact.test/upload/${input.key}` };
  }
  seed(input: Omit<ArtifactObjectMetadata, "etag" | "uploadedAt"> & Partial<Pick<ArtifactObjectMetadata, "etag" | "uploadedAt">> & { bytes?: Uint8Array }): void {
    this.objects.set(input.key, { ...input, etag: input.etag ?? randomUUID(), uploadedAt: input.uploadedAt ?? new Date().toISOString() });
  }
}
