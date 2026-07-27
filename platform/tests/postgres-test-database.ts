import { drizzle } from "drizzle-orm/node-postgres";
import { Pool } from "pg";

import * as schema from "@/platform/database/schema";
import { parsePostgresTestTarget } from "../scripts/postgres-test-safety";

export function openPostgresTestDatabase() {
  const target = parsePostgresTestTarget(
    process.env.FILECHEAP_POSTGRES_TEST_URL,
    {
      allowRemote:
        process.env.FILECHEAP_ALLOW_REMOTE_TEST_DATABASE?.toLowerCase() ===
        "true",
    },
  );
  const pool = new Pool({ connectionString: target.databaseUrl, max: 4 });
  return {
    database: drizzle(pool, { schema }),
    pool,
  };
}

export type PostgresTestDatabase = ReturnType<
  typeof openPostgresTestDatabase
>["database"];

export async function truncatePostgresTestData(
  harness: ReturnType<typeof openPostgresTestDatabase>,
): Promise<void> {
  // The URL passed through the safety parser before this Pool was created.
  // CASCADE includes future FK dependants while the explicit table list keeps
  // this cleanup scoped to product data in an unmistakable test database.
  await harness.pool.query(`
    TRUNCATE TABLE
      private_activity_events,
      private_retention_runs,
      inbound_email_replays,
      console_rate_limits,
      console_refresh_tokens,
      console_sessions,
      console_device_families,
      console_verification_deliveries,
      console_authorizations,
      artifact_runs,
      artifact_objects,
      artifacts,
      console_users
    CASCADE
  `);
}
