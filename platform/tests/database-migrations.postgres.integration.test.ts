import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { eq } from "drizzle-orm";

import {
  artifacts,
  consoleUsers,
} from "@/platform/database/schema";
import {
  openPostgresTestDatabase,
  truncatePostgresTestData,
} from "./postgres-test-database";

const databaseUrl = process.env.FILECHEAP_POSTGRES_TEST_URL;
const expectedTables = [
  "artifact_objects",
  "artifact_runs",
  "artifacts",
  "console_authorizations",
  "console_device_families",
  "console_rate_limits",
  "console_refresh_tokens",
  "console_sessions",
  "console_users",
  "console_verification_deliveries",
  "inbound_email_replays",
  "private_activity_events",
  "private_retention_runs",
];
const migrationJournal = JSON.parse(
  readFileSync(`${process.cwd()}/drizzle/meta/_journal.json`, "utf8"),
) as { entries: Array<{ idx: number; tag: string }> };

describe.skipIf(!databaseUrl)("PostgreSQL migrations from an empty database", () => {
  let harness: ReturnType<typeof openPostgresTestDatabase>;

  beforeAll(() => {
    harness = openPostgresTestDatabase();
  });

  afterAll(async () => {
    await harness.pool.end();
  });

  test("applies every journal entry and creates the complete current schema", async () => {
    const migrationResult = await harness.pool.query<{ count: number }>(`
      SELECT count(*)::integer AS count
      FROM drizzle.__drizzle_migrations
    `);
    const tableResult = await harness.pool.query<{ table_name: string }>(`
      SELECT table_name
      FROM information_schema.tables
      WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
      ORDER BY table_name
    `);

    expect(migrationResult.rows[0]?.count).toBe(migrationJournal.entries.length);
    expect(tableResult.rows.map((row) => row.table_name)).toEqual(expectedTables);
  });

  test("enforces the final owner boundary in PostgreSQL itself", async () => {
    const columnResult = await harness.pool.query<{ is_nullable: string }>(`
      SELECT is_nullable
      FROM information_schema.columns
      WHERE table_schema = 'public'
        AND table_name = 'artifacts'
        AND column_name = 'owner_account_id'
    `);
    const foreignKeyResult = await harness.pool.query<{
      constraint_name: string;
    }>(`
      SELECT constraint_name
      FROM information_schema.table_constraints
      WHERE table_schema = 'public'
        AND table_name = 'artifacts'
        AND constraint_type = 'FOREIGN KEY'
    `);

    expect(columnResult.rows).toEqual([{ is_nullable: "NO" }]);
    expect(foreignKeyResult.rows.map((row) => row.constraint_name)).toContain(
      "artifacts_owner_account_id_console_users_id_fk",
    );
  });

  test("adds an online-compatible receipt envelope while retaining the raw column", async () => {
    const columnResult = await harness.pool.query<{
      column_name: string;
      is_nullable: string;
    }>(`
      SELECT column_name, is_nullable
      FROM information_schema.columns
      WHERE table_schema = 'public'
        AND table_name = 'artifacts'
        AND column_name IN (
          'plan_token',
          'plan_receipt_scheme',
          'plan_receipt_kid',
          'plan_receipt_nonce',
          'plan_receipt_lookup'
        )
      ORDER BY column_name
    `);
    const indexResult = await harness.pool.query<{
      indexdef: string;
      indexname: string;
    }>(`
      SELECT indexname, indexdef
      FROM pg_indexes
      WHERE schemaname = 'public'
        AND tablename = 'artifacts'
        AND indexname = 'artifacts_plan_receipt_lookup_unique'
    `);

    expect(columnResult.rows).toEqual([
      { column_name: "plan_receipt_kid", is_nullable: "YES" },
      { column_name: "plan_receipt_lookup", is_nullable: "YES" },
      { column_name: "plan_receipt_nonce", is_nullable: "YES" },
      { column_name: "plan_receipt_scheme", is_nullable: "YES" },
      { column_name: "plan_token", is_nullable: "NO" },
    ]);
    expect(indexResult.rows[0]?.indexdef).toContain(
      "UNIQUE INDEX artifacts_plan_receipt_lookup_unique",
    );
    expect(indexResult.rows[0]?.indexdef).toContain(
      "WHERE (plan_receipt_lookup IS NOT NULL)",
    );
  });

  test("executes a real Drizzle node-postgres round trip", async () => {
    const now = new Date("2026-07-26T18:00:00.000Z");
    const ownerAccountId = "acc_postgres_test_owner";
    const artifactId = "art_postgres_migration_test";
    const database = harness.database;

    await database.insert(consoleUsers).values({
      createdAt: now,
      email: "postgres-test@example.invalid",
      id: ownerAccountId,
      updatedAt: now,
    });

    try {
      await database.insert(artifacts).values({
        artifactId,
        contentType: "application/zstd",
        createdAt: now,
        kind: "chalupa.log-bundle",
        ownerAccountId,
        planExpiresAt: new Date("2026-07-26T18:15:00.000Z"),
        planToken: "postgres-test-plan-token",
        producer: { tool: "chalupa", version: "test" },
        sha256: "a".repeat(64),
        sizeBytes: 128,
        state: "planned",
        verification: "server-sha256",
      });

      const selected = await database
        .select()
        .from(artifacts)
        .where(eq(artifacts.artifactId, artifactId));
      expect(selected[0]).toMatchObject({
        artifactId,
        ownerAccountId,
        sizeBytes: 128,
      });
    } finally {
      await truncatePostgresTestData(harness);
    }
  });
});
