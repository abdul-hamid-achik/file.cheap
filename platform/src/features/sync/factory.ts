import { CatalogRepository } from "@/features/catalog/catalog";
import { SyncService } from "@/features/sync/sync-service";
import { getObjectStore } from "@/platform/storage/factory";
import { getConfig } from "@/shared/config/env";

let syncService: SyncService | undefined;

export function getSyncService(): SyncService {
  if (syncService) {
    return syncService;
  }

  const store = getObjectStore();
  syncService = new SyncService(
    store,
    new CatalogRepository(store),
    getConfig().signingSecret,
  );
  return syncService;
}

export function resetSyncServiceForTests(): void {
  syncService = undefined;
}
