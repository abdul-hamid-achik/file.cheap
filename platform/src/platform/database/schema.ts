import { sql } from "drizzle-orm";
import { check, index, integer, jsonb, pgTable, text, timestamp, uniqueIndex } from "drizzle-orm/pg-core";

export const artifacts = pgTable("artifacts", {
  artifactId: text("artifact_id").primaryKey(),
  kind: text("kind").notNull(),
  producer: jsonb("producer").notNull(),
  sha256: text("sha256").notNull(),
  sizeBytes: integer("size_bytes").notNull(),
  contentType: text("content_type").notNull(),
  state: text("state").notNull(),
  verification: text("verification").notNull(),
  planToken: text("plan_token").notNull(),
  planExpiresAt: timestamp("plan_expires_at", { withTimezone: true }).notNull(),
  expiresAt: timestamp("expires_at", { withTimezone: true }),
  committedAt: timestamp("committed_at", { withTimezone: true }),
  deletingAt: timestamp("deleting_at", { withTimezone: true }),
  deletedAt: timestamp("deleted_at", { withTimezone: true }),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull(),
}, (table) => [
  check(
    "artifacts_state_check",
    sql`${table.state} in ('planned', 'committed', 'deleting', 'deleted')`,
  ),
  check(
    "artifacts_verification_check",
    sql`${table.verification} in ('server-sha256')`,
  ),
  check(
    "artifacts_sha256_check",
    sql`${table.sha256} ~ '^[a-f0-9]{64}$'`,
  ),
  check(
    "artifacts_size_check",
    sql`${table.sizeBytes} > 0 and ${table.sizeBytes} <= 2097152`,
  ),
  check(
    "artifacts_expiry_check",
    sql`${table.expiresAt} is null or ${table.expiresAt} > ${table.createdAt}`,
  ),
  uniqueIndex("artifacts_plan_token_unique").on(table.planToken),
  index("artifacts_retention_index").on(table.state, table.expiresAt),
]);

export const artifactObjects = pgTable("artifact_objects", {
  id: text("id").primaryKey(),
  artifactId: text("artifact_id").notNull().references(() => artifacts.artifactId, { onDelete: "cascade" }),
  ordinal: integer("ordinal").notNull(),
  objectKey: text("object_key").notNull(),
  sha256: text("sha256").notNull(),
  sizeBytes: integer("size_bytes").notNull(),
  contentType: text("content_type").notNull(),
  etag: text("etag"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull(),
}, (table) => [
  check("artifact_objects_ordinal_check", sql`${table.ordinal} >= 0`),
  check(
    "artifact_objects_size_check",
    sql`${table.sizeBytes} > 0 and ${table.sizeBytes} <= 2097152`,
  ),
  uniqueIndex("artifact_objects_key_unique").on(table.objectKey),
  uniqueIndex("artifact_objects_artifact_ordinal_unique").on(
    table.artifactId,
    table.ordinal,
  ),
]);

// Pseudonymized, joinable SHA-256 digests—not provider identifiers or
// addresses—make retries durable without turning this ledger into an inbox.
export const inboundEmailReplays = pgTable("inbound_email_replays", {
  id: text("id").primaryKey(),
  svixIdSha256: text("svix_id_sha256").notNull(),
  emailIdSha256: text("email_id_sha256").notNull(),
  status: text("status").notNull(),
  attempts: integer("attempts").notNull(),
  leaseToken: text("lease_token"),
  processingLeaseExpiresAt: timestamp("processing_lease_expires_at", {
    withTimezone: true,
  }),
  forwardedAt: timestamp("forwarded_at", { withTimezone: true }),
  ambiguousAt: timestamp("ambiguous_at", { withTimezone: true }),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull(),
}, (table) => [
  check(
    "inbound_email_replays_status_check",
    sql`${table.status} in ('processing', 'forwarded', 'ignored', 'ambiguous', 'rejected')`,
  ),
  check("inbound_email_replays_attempts_check", sql`${table.attempts} > 0`),
  check(
    "inbound_email_replays_attempts_upper_bound_check",
    sql`${table.attempts} <= 8`,
  ),
  check(
    "inbound_email_replays_svix_digest_check",
    sql`${table.svixIdSha256} ~ '^[a-f0-9]{64}$'`,
  ),
  check(
    "inbound_email_replays_email_digest_check",
    sql`${table.emailIdSha256} ~ '^[a-f0-9]{64}$'`,
  ),
  check(
    "inbound_email_replays_expiry_check",
    sql`${table.expiresAt} > ${table.createdAt}`,
  ),
  check(
    "inbound_email_replays_processing_lease_check",
    sql`(${table.status} = 'processing') = (${table.leaseToken} is not null and ${table.processingLeaseExpiresAt} is not null)`,
  ),
  check(
    "inbound_email_replays_forwarded_check",
    sql`(${table.status} = 'forwarded') = (${table.forwardedAt} is not null)`,
  ),
  check(
    "inbound_email_replays_ambiguous_check",
    sql`(${table.status} = 'ambiguous') = (${table.ambiguousAt} is not null)`,
  ),
  uniqueIndex("inbound_email_replays_svix_digest_unique").on(
    table.svixIdSha256,
  ),
  uniqueIndex("inbound_email_replays_email_digest_unique").on(
    table.emailIdSha256,
  ),
  index("inbound_email_replays_expiry_index").on(table.expiresAt),
]);
