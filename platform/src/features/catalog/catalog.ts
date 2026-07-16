import { z } from "zod";

import {
  protocolV1DateTimeSchema,
  protocolV1MaximumCatalogEntries,
  protocolV1MaxObjectBytes,
  protocolV1ObjectKeySchema,
  sha256Schema,
  stashContentType,
  stashIdSchema,
} from "@/features/sync/contracts";
import {
  throwIfStorageOperationAborted,
  type ObjectStore,
} from "@/platform/storage/object-store";
import {
  CatalogPreconditionError,
  PlatformError,
} from "@/shared/errors/platform-error";

const catalogRetryDeadlineMilliseconds = 15_000;

export type CatalogRetryPolicy = {
  deadlineMilliseconds: number;
  delay: (attempt: number, signal?: AbortSignal) => Promise<void>;
  now: () => number;
};

const defaultRetryPolicy: CatalogRetryPolicy = {
  deadlineMilliseconds: catalogRetryDeadlineMilliseconds,
  delay: async (attempt, signal) => {
    const jitterCeiling = Math.min(100, 2 ** Math.min(attempt, 7));
    const delayMilliseconds = 1 + Math.floor(Math.random() * jitterCeiling);
    await abortableDelay(delayMilliseconds, signal);
  },
  now: () => Date.now(),
};

export const cloudStashSchema = z
  .object({
    committedAt: protocolV1DateTimeSchema,
    contentType: z.literal(stashContentType),
    etag: z.string().min(1).max(512),
    objectKey: protocolV1ObjectKeySchema,
    sha256: sha256Schema,
    sizeBytes: z.number().int().positive().max(protocolV1MaxObjectBytes),
    stashId: stashIdSchema,
    storageVerification: z
      .enum(["presence-size-etag", "server-sha256"])
      .default("presence-size-etag"),
  })
  .strict();

const catalogSchema = z
  .object({
    revision: z.number().int().nonnegative(),
    schemaVersion: z.literal(1),
    stashes: z.record(stashIdSchema, cloudStashSchema),
    updatedAt: protocolV1DateTimeSchema,
  })
  .strict()
  .superRefine((catalog, context) => {
    const stashes = Object.entries(catalog.stashes);
    if (stashes.length > protocolV1MaximumCatalogEntries) {
      context.addIssue({
        code: "custom",
        message: "catalog exceeds the protocol-v1 entry limit",
        path: ["stashes"],
      });
    }
    for (const [stashId, stash] of stashes) {
      if (stashId !== stash.stashId) {
        context.addIssue({
          code: "custom",
          message: "catalog record key must match the embedded stash ID",
          path: ["stashes", stashId, "stashId"],
        });
      }
    }
  });

export type CloudStash = z.infer<typeof cloudStashSchema>;
type Catalog = z.infer<typeof catalogSchema>;

type LoadedCatalog = {
  catalog: Catalog;
  etag?: string;
};

export class CatalogRepository {
  private updateTail: Promise<void> = Promise.resolve();

  constructor(
    private readonly store: ObjectStore,
    private readonly key = "v1/workspaces/default/catalog/v1.json",
    private readonly retryPolicy: CatalogRetryPolicy = defaultRetryPolicy,
  ) {}

  async list(signal?: AbortSignal): Promise<CloudStash[]> {
    const { catalog } = await this.load(signal);
    return Object.values(catalog.stashes).sort((left, right) =>
      right.committedAt.localeCompare(left.committedAt),
    );
  }

  async find(stashId: string, signal?: AbortSignal): Promise<CloudStash | null> {
    const { catalog } = await this.load(signal);
    return catalog.stashes[stashId] ?? null;
  }

  async findForPlan(
    stashId: string,
    signal?: AbortSignal,
  ): Promise<CloudStash | null> {
    const { catalog } = await this.load(signal);
    const existing = catalog.stashes[stashId] ?? null;
    if (
      !existing &&
      Object.keys(catalog.stashes).length >= protocolV1MaximumCatalogEntries
    ) {
      throw catalogCapacityReached();
    }
    return existing;
  }

  async commit(stash: CloudStash, signal?: AbortSignal): Promise<CloudStash> {
    return this.serialized(async () => {
      throwIfStorageOperationAborted(signal);
      const deadline =
        this.retryPolicy.now() + this.retryPolicy.deadlineMilliseconds;
      let attempt = 0;

      while (true) {
        const loaded = await this.load(signal);
        const existing = loaded.catalog.stashes[stash.stashId];
        if (existing) {
          if (!sameStashIdentity(existing, stash)) {
            throw new PlatformError({
              code: "stash_conflict",
              detail: `Stash ${stash.stashId} is already committed to different content.`,
              status: 409,
              title: "Stash conflict",
            });
          }
          return existing;
        }
        if (
          Object.keys(loaded.catalog.stashes).length >=
          protocolV1MaximumCatalogEntries
        ) {
          throw catalogCapacityReached();
        }

        const nextCatalog: Catalog = {
          revision: loaded.catalog.revision + 1,
          schemaVersion: 1,
          stashes: {
            ...loaded.catalog.stashes,
            [stash.stashId]: stash,
          },
          updatedAt: new Date().toISOString(),
        };

        try {
          await this.store.writeText(
            {
              body: `${JSON.stringify(nextCatalog, null, 2)}\n`,
              expectedEtag: loaded.etag,
              key: this.key,
            },
            signal,
          );
          return stash;
        } catch (error) {
          if (!(error instanceof CatalogPreconditionError)) {
            throw error;
          }
          if (this.retryPolicy.now() >= deadline) {
            throw catalogContention();
          }
          await this.retryPolicy.delay(attempt, signal);
          attempt += 1;
        }
      }
    }, signal);
  }

  private async load(signal?: AbortSignal): Promise<LoadedCatalog> {
    throwIfStorageOperationAborted(signal);
    const stored = await this.store.readText(this.key, signal);
    if (!stored) {
      return {
        catalog: {
          revision: 0,
          schemaVersion: 1,
          stashes: {},
          updatedAt: new Date(0).toISOString(),
        },
      };
    }

    return {
      catalog: catalogSchema.parse(JSON.parse(stored.body)),
      etag: stored.etag,
    };
  }

  private async serialized<T>(
    operation: () => Promise<T>,
    signal?: AbortSignal,
  ): Promise<T> {
    const previous = this.updateTail;
    const queued = previous.then(async () => {
      throwIfStorageOperationAborted(signal);
      return operation();
    });
    this.updateTail = queued.then(
      () => undefined,
      () => undefined,
    );
    return await abortablePromise(queued, signal);
  }
}

async function abortablePromise<T>(
  promise: Promise<T>,
  signal?: AbortSignal,
): Promise<T> {
  throwIfStorageOperationAborted(signal);
  if (!signal) return promise;

  return await new Promise<T>((resolve, reject) => {
    const cleanup = (): void => signal.removeEventListener("abort", abort);
    const abort = (): void => {
      cleanup();
      try {
        throwIfStorageOperationAborted(signal);
      } catch (error) {
        reject(error);
      }
    };
    signal.addEventListener("abort", abort, { once: true });
    if (signal.aborted) {
      abort();
      return;
    }
    void promise.then(
      (value) => {
        cleanup();
        resolve(value);
      },
      (error: unknown) => {
        cleanup();
        reject(error);
      },
    );
  });
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

function sameStashIdentity(left: CloudStash, right: CloudStash): boolean {
  return (
    left.stashId === right.stashId &&
    left.contentType === right.contentType &&
    left.etag === right.etag &&
    left.objectKey === right.objectKey &&
    left.sha256 === right.sha256 &&
    left.sizeBytes === right.sizeBytes &&
    left.storageVerification === right.storageVerification
  );
}

function catalogContention(): PlatformError {
  return new PlatformError({
    code: "catalog_busy",
    detail: "The catalog remained busy. Retry the operation.",
    status: 503,
    title: "Catalog busy",
  });
}

function catalogCapacityReached(): PlatformError {
  return new PlatformError({
    code: "catalog_capacity_reached",
    detail:
      "The protocol-v1 catalog reached its safe entry limit. Pagination is required before adding another stash.",
    status: 409,
    title: "Catalog capacity reached",
  });
}
