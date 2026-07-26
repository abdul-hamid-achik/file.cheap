import { neon } from "@neondatabase/serverless";
import { z } from "zod";

const input = z.object({
  FILECHEAP_OWNER_ACCOUNT_ID: z.string().regex(/^acc_[A-Za-z0-9_-]{8,64}$/u),
  FILECHEAP_OWNER_EMAIL: z.string().trim().email().max(320),
  MIGRATIONS_DATABASE_URL: z.string().min(1),
}).parse(process.env);

const sql = neon(input.MIGRATIONS_DATABASE_URL);
const now = new Date().toISOString();
const [users, artifacts] = await sql.transaction([
  sql`
    INSERT INTO console_users (id, email, created_at, updated_at)
    VALUES (
      ${input.FILECHEAP_OWNER_ACCOUNT_ID},
      ${input.FILECHEAP_OWNER_EMAIL.toLowerCase()},
      ${now}::timestamptz,
      ${now}::timestamptz
    )
    ON CONFLICT (id) DO UPDATE
    SET email = EXCLUDED.email, updated_at = EXCLUDED.updated_at
    RETURNING id
  `,
  sql`
    UPDATE artifacts
    SET owner_account_id = ${input.FILECHEAP_OWNER_ACCOUNT_ID}
    WHERE owner_account_id IS NULL
    RETURNING artifact_id
  `,
]);

if (users.length !== 1) throw new Error("Owner account backfill did not produce exactly one account");
console.log(JSON.stringify({ artifactsBackfilled: artifacts.length, ownerAccountId: input.FILECHEAP_OWNER_ACCOUNT_ID }));
