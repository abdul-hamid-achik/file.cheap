import { getConfig } from "@/shared/config/env";
import { LocalObjectStore } from "@/platform/storage/local-object-store";
import type { ObjectStore } from "@/platform/storage/object-store";
import { VercelBlobStore } from "@/platform/storage/vercel-blob-store";

let objectStore: ObjectStore | undefined;

export function getObjectStore(): ObjectStore {
  if (objectStore) {
    return objectStore;
  }

  objectStore =
    getConfig().storageDriver === "vercel-blob"
      ? new VercelBlobStore()
      : new LocalObjectStore();
  return objectStore;
}

export function resetObjectStoreForTests(): void {
  objectStore = undefined;
}
