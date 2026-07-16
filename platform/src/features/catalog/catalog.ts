import { z } from "zod";

import { stashContentType } from "@/features/sync/contracts";
import { CatalogPreconditionError } from "@/shared/errors/platform-error";
import { PlatformError } from "@/shared/errors/platform-error";
import type { ObjectStore } from "@/platform/storage/object-store";

const catalogRetryDeadlineMilliseconds = 15_000;

export type CatalogRetryPolicy = {
  deadlineMilliseconds: number;
  delay: (attempt: number) => Promise<void>;
  now: () => number;
};

const defaultRetryPolicy: CatalogRetryPolicy = {
  deadlineMilliseconds: catalogRetryDeadlineMilliseconds,
  delay: async (attempt) => {
    const jitterCeiling = Math.min(100, 2 ** Math.min(attempt, 7));
    const delayMilliseconds = 1 + Math.floor(Math.random() * jitterCeiling);
    await new Promise((resolve) => setTimeout(resolve, delayMilliseconds));
  },
  now: () => Date.now(),
};

export const cloudStashSchema = z.object({
  committedAt: z.iso.datetime(),
  contentType: z.literal(stashContentType),
  etag: z.string().min(1),
  objectKey: z.string().min(1),
  sha256: z.string().regex(/^[a-f0-9]{64}$/),
  sizeBytes: z.number().int().nonnegative(),
  stashId: z.string().min(1),
  storageVerification: z
    .enum(["presence-size-etag", "server-sha256"])
    .default("presence-size-etag"),
});

const catalogSchema = z.object({
  revision: z.number().int().nonnegative(),
  schemaVersion: z.literal(1),
  stashes: z.record(z.string(), cloudStashSchema),
  updatedAt: z.iso.datetime(),
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

  async list(): Promise<CloudStash[]> {
    const { catalog } = await this.load();
    return Object.values(catalog.stashes).sort((left, right) =>
      right.committedAt.localeCompare(left.committedAt),
    );
  }

  async find(stashId: string): Promise<CloudStash | null> {
    const { catalog } = await this.load();
    return catalog.stashes[stashId] ?? null;
  }

  async commit(stash: CloudStash): Promise<CloudStash> {
    return this.serialized(async () => {
      const deadline =
        this.retryPolicy.now() + this.retryPolicy.deadlineMilliseconds;
      let attempt = 0;

      while (true) {
        const loaded = await this.load();
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
          await this.store.writeText({
            body: `${JSON.stringify(nextCatalog, null, 2)}\n`,
            expectedEtag: loaded.etag,
            key: this.key,
          });
          return stash;
        } catch (error) {
          if (!(error instanceof CatalogPreconditionError)) {
            throw error;
          }
          if (this.retryPolicy.now() >= deadline) {
            throw catalogContention();
          }
          await this.retryPolicy.delay(attempt);
          attempt += 1;
        }
      }
    });
  }

  private async load(): Promise<LoadedCatalog> {
    const stored = await this.store.readText(this.key);
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

  private async serialized<T>(operation: () => Promise<T>): Promise<T> {
    const previous = this.updateTail;
    let release: () => void = () => undefined;
    this.updateTail = new Promise<void>((resolve) => {
      release = resolve;
    });

    await previous;
    try {
      return await operation();
    } finally {
      release();
    }
  }
}

function sameStashIdentity(left: CloudStash, right: CloudStash): boolean {
  return (
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
