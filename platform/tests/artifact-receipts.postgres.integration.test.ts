import { createHash } from "node:crypto";
import {
  afterAll,
  beforeAll,
  beforeEach,
  describe,
  expect,
  test,
} from "bun:test";
import { eq } from "drizzle-orm";

import type {
  ArtifactPlanResponse,
  ArtifactPlanResult,
} from "@/features/artifacts/contracts";
import {
  derivePlanReceiptLookup,
  legacyPlanReceiptLookupSchemeV1,
  PlanReceiptKeyring,
} from "@/features/artifacts/plan-receipts";
import { ArtifactService } from "@/features/artifacts/service";
import { InMemoryArtifactObjectStore } from "@/platform/artifacts/in-memory-object-store";
import { getDatabase } from "@/platform/database/client";
import { DrizzleArtifactRepository } from "@/platform/database/repository";
import { artifacts, consoleUsers } from "@/platform/database/schema";
import {
  openPostgresTestDatabase,
  truncatePostgresTestData,
} from "./postgres-test-database";

const databaseUrl = process.env.FILECHEAP_POSTGRES_TEST_URL;
const ownerAccountId = "acc_receipts_postgres_owner";
const bytes = new TextEncoder().encode("postgres receipt artifact");
const sha256 = createHash("sha256").update(bytes).digest("hex");

describe.skipIf(!databaseUrl)("artifact receipt PostgreSQL repository", () => {
  let harness: ReturnType<typeof openPostgresTestDatabase>;
  let now: Date;

  beforeAll(() => {
    harness = openPostgresTestDatabase();
  });

  beforeEach(async () => {
    await truncatePostgresTestData(harness);
    now = new Date("2026-07-26T18:00:00.000Z");
    await harness.database.insert(consoleUsers).values({
      createdAt: now,
      email: "receipt-owner@example.invalid",
      id: ownerAccountId,
      updatedAt: now,
    });
  });

  afterAll(async () => {
    await truncatePostgresTestData(harness);
    await harness.pool.end();
  });

  test("coalesces concurrent plans across an overlapping key rotation and replays one UUID", async () => {
    const repository = artifactRepository();
    const store = new InMemoryArtifactObjectStore();
    const oldService = new ArtifactService(
      store,
      repository,
      rotatingKeyring("old"),
      () => now,
    );
    const currentService = new ArtifactService(
      store,
      artifactRepository(),
      rotatingKeyring("current"),
      () => now,
    );
    const input = planInput("123e4567-e89b-42d3-a456-426614174040");

    const plans = await Promise.all(
      Array.from({ length: 8 }, (_, index) =>
        (index % 2 === 0 ? oldService : currentService).plan(
          input,
          undefined,
          ownerAccountId,
        ),
      ),
    );
    const freshPlans = plans.map(requirePlanned);
    expect(new Set(freshPlans.map((plan) => plan.receipt)).size).toBe(1);
    expect(new Set(freshPlans.map((plan) => plan.artifact.artifactId)).size)
      .toBe(1);
    const receipt = freshPlans[0]!.receipt;
    const row = (await harness.database.select().from(artifacts))[0];
    expect(row).toMatchObject({
      planReceiptScheme: "hmac-sha256-v1",
      planToken: receipt,
      state: "planned",
    });
    expect(row?.planReceiptKid === "current" || row?.planReceiptKid === "old")
      .toBe(true);
    expect(row?.planReceiptLookup).toHaveLength(43);
    expect(row?.planReceiptNonce).toHaveLength(43);

    store.seed({
      bytes,
      contentType: input.contentType,
      key: new URL(freshPlans[0]!.upload.url).pathname.replace(
        /^\/upload\//u,
        "",
      ),
      sizeBytes: bytes.byteLength,
    });
    const committed = await Promise.all([
      oldService.commit(receipt),
      currentService.commit(receipt),
    ]);
    expect(committed.map((value) => value.artifact.state)).toEqual([
      "committed",
      "committed",
    ]);

    now = new Date("2026-07-26T18:16:00.000Z");
    await expect(currentService.commit(receipt)).rejects.toMatchObject({
      code: "invalid_receipt",
      status: 400,
    });
  });

  test("does not downgrade a corrupted HMAC row to its dual-written raw token", async () => {
    const store = new InMemoryArtifactObjectStore();
    const service = new ArtifactService(
      store,
      artifactRepository(),
      rotatingKeyring("current"),
      () => now,
    );
    const plan = requirePlanned(await service.plan(
      planInput("123e4567-e89b-42d3-a456-426614174041"),
      undefined,
      ownerAccountId,
    ));
    await harness.database
      .update(artifacts)
      .set({ planReceiptLookup: Buffer.alloc(32, 0x7f).toString("base64url") })
      .where(eq(artifacts.artifactId, plan.artifact.artifactId));

    await expect(service.commit(plan.receipt)).rejects.toMatchObject({
      code: "invalid_receipt",
      status: 400,
    });
    expect((await harness.database.select().from(artifacts))[0]?.planToken)
      .toBe(plan.receipt);
  });

  test("uses raw lookup only for a row whose receipt envelope is totally legacy", async () => {
    const store = new InMemoryArtifactObjectStore();
    const repository = artifactRepository();
    const service = new ArtifactService(
      store,
      repository,
      rotatingKeyring("current"),
      () => now,
    );
    const plan = requirePlanned(await service.plan(
      planInput("123e4567-e89b-42d3-a456-426614174042"),
      undefined,
      ownerAccountId,
    ));
    await harness.database.update(artifacts).set({
      planReceiptKid: null,
      planReceiptLookup: null,
      planReceiptNonce: null,
      planReceiptScheme: null,
    }).where(eq(artifacts.artifactId, plan.artifact.artifactId));
    const record = await repository.find(plan.artifact.artifactId);
    if (!record) throw new Error("Expected a legacy PostgreSQL record");
    store.seed({
      bytes,
      contentType: record.contentType,
      key: record.objectKey,
      sizeBytes: bytes.byteLength,
    });

    expect((await service.commit(plan.receipt)).artifact.state).toBe("committed");
  });

  test("accepts a lookup-only keyed legacy backfill", async () => {
    const store = new InMemoryArtifactObjectStore();
    const keyring = rotatingKeyring("current");
    const repository = artifactRepository();
    const service = new ArtifactService(store, repository, keyring, () => now);
    const plan = requirePlanned(await service.plan(
      planInput("123e4567-e89b-42d3-a456-426614174043"),
      undefined,
      ownerAccountId,
    ));
    const lookup = derivePlanReceiptLookup(keyring, "current", plan.receipt);
    await harness.database.update(artifacts).set({
      planReceiptKid: "current",
      planReceiptLookup: lookup,
      planReceiptNonce: null,
      planReceiptScheme: legacyPlanReceiptLookupSchemeV1,
    }).where(eq(artifacts.artifactId, plan.artifact.artifactId));
    const record = await repository.find(plan.artifact.artifactId);
    if (!record) throw new Error("Expected a keyed legacy PostgreSQL record");
    store.seed({
      bytes,
      contentType: record.contentType,
      key: record.objectKey,
      sizeBytes: bytes.byteLength,
    });

    expect((await service.commit(plan.receipt)).artifact.state).toBe("committed");
  });

  function artifactRepository(): DrizzleArtifactRepository {
    return new DrizzleArtifactRepository(
      harness.database as unknown as ReturnType<typeof getDatabase>,
    );
  }
});

function planInput(idempotencyKey: string) {
  return {
    contentType: "application/zstd",
    idempotencyKey,
    kind: "chalupa.log-chunk",
    producer: { tool: "chalupa" },
    sha256,
    sizeBytes: bytes.byteLength,
  } as const;
}

function requirePlanned(result: ArtifactPlanResult): ArtifactPlanResponse {
  if (!("receipt" in result) || !("upload" in result)) {
    throw new Error("Expected a fresh PostgreSQL artifact plan");
  }
  return result;
}

function rotatingKeyring(activeKid: "current" | "old"): PlanReceiptKeyring {
  return new PlanReceiptKeyring({
    activeKid,
    keys: [
      {
        kid: "current",
        lookupKey: Buffer.alloc(32, 4),
        signingKey: Buffer.alloc(32, 3),
      },
      {
        kid: "old",
        lookupKey: Buffer.alloc(32, 2),
        signingKey: Buffer.alloc(32, 1),
      },
    ],
  });
}
