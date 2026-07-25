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
