import { sql } from "drizzle-orm";
import { check, index, integer, jsonb, pgTable, text, timestamp, uniqueIndex } from "drizzle-orm/pg-core";

// Console identity is deliberately separate from service credentials. The
// first release is owner-allowlisted, but ownership is a database invariant so
// future account expansion cannot accidentally expose legacy global rows.
export const consoleUsers = pgTable("console_users", {
  id: text("id").primaryKey(),
  email: text("email").notNull(),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull(),
}, (table) => [
  uniqueIndex("console_users_email_unique").on(table.email),
]);

export const artifacts = pgTable("artifacts", {
  artifactId: text("artifact_id").primaryKey(),
  ownerAccountId: text("owner_account_id").notNull().references(() => consoleUsers.id, { onDelete: "restrict" }),
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
  // Global platform ceiling. Keep the literal in sync with
  // `maximumArtifactBytes` in src/shared/config/limits.ts; the migration-graph
  // test asserts the two agree. Per-producer quotas are enforced at runtime and
  // are always at or below this value.
  check(
    "artifacts_size_check",
    sql`${table.sizeBytes} > 0 and ${table.sizeBytes} <= 67108864`,
  ),
  check(
    "artifacts_expiry_check",
    sql`${table.expiresAt} is null or ${table.expiresAt} > ${table.createdAt}`,
  ),
  uniqueIndex("artifacts_plan_token_unique").on(table.planToken),
  index("artifacts_retention_index").on(table.state, table.expiresAt),
  index("artifacts_owner_created_index").on(table.ownerAccountId, table.createdAt, table.artifactId),
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
    sql`${table.sizeBytes} > 0 and ${table.sizeBytes} <= 67108864`,
  ),
  uniqueIndex("artifact_objects_key_unique").on(table.objectKey),
  uniqueIndex("artifact_objects_artifact_ordinal_unique").on(
    table.artifactId,
    table.ordinal,
  ),
]);

// Metadata-only projection emitted by a trusted local detector and bound to
// the immutable artifact plan. The hosted service never opens the archive to
// manufacture this data, and the projection becomes visible only after the
// parent artifact is committed.
export const artifactRuns = pgTable("artifact_runs", {
  artifactId: text("artifact_id").primaryKey().references(() => artifacts.artifactId, { onDelete: "cascade" }),
  ownerAccountId: text("owner_account_id").notNull().references(() => consoleUsers.id, { onDelete: "restrict" }),
  schemaVersion: integer("schema_version").notNull(),
  runIndexSha256: text("run_index_sha256").notNull(),
  sourceSha256: text("source_sha256").notNull(),
  detectorName: text("detector_name").notNull(),
  detectorVersion: text("detector_version").notNull(),
  producerTool: text("producer_tool").notNull(),
  nativeSchema: text("native_schema").notNull(),
  nativeRunId: text("native_run_id").notNull(),
  seriesKey: text("series_key").notNull(),
  specName: text("spec_name"),
  status: text("status").notNull(),
  health: text("health").notNull(),
  startedAt: timestamp("started_at", { withTimezone: true }),
  endedAt: timestamp("ended_at", { withTimezone: true }),
  durationMs: integer("duration_ms"),
  environment: text("environment"),
  backend: text("backend"),
  exitCode: integer("exit_code"),
  errorKind: text("error_kind"),
  stepCount: integer("step_count").notNull(),
  outcomeCount: integer("outcome_count").notNull(),
  artifactCount: integer("artifact_count").notNull(),
  healthDeclared: integer("health_declared").notNull(),
  healthPresent: integer("health_present").notNull(),
  healthEmpty: integer("health_empty").notNull(),
  healthMissing: integer("health_missing").notNull(),
  healthChanged: integer("health_changed").notNull(),
  healthReasons: jsonb("health_reasons").notNull(),
  outcomes: jsonb("outcomes").notNull(),
  evidence: jsonb("evidence").notNull(),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull(),
}, (table) => [
  check("artifact_runs_schema_version_check", sql`${table.schemaVersion} = 1`),
  check("artifact_runs_index_digest_check", sql`${table.runIndexSha256} ~ '^[a-f0-9]{64}$'`),
  check("artifact_runs_source_digest_check", sql`${table.sourceSha256} ~ '^[a-f0-9]{64}$'`),
  check("artifact_runs_detector_check", sql`${table.detectorName} in ('cairntrace-run', 'glyphrun-run')`),
  check("artifact_runs_status_check", sql`${table.status} in ('queued', 'running', 'passed', 'failed', 'errored', 'cancelled', 'incomplete', 'unknown')`),
  check("artifact_runs_health_check", sql`${table.health} in ('ok', 'degraded', 'incomplete', 'unknown')`),
  check("artifact_runs_duration_check", sql`${table.durationMs} is null or ${table.durationMs} >= 0`),
  check("artifact_runs_time_check", sql`${table.startedAt} is null or ${table.endedAt} is null or ${table.endedAt} >= ${table.startedAt}`),
  check("artifact_runs_counts_check", sql`${table.stepCount} >= 0 and ${table.outcomeCount} >= 0 and ${table.artifactCount} >= 0`),
  check("artifact_runs_health_counts_check", sql`${table.healthDeclared} >= 0 and ${table.healthPresent} >= 0 and ${table.healthEmpty} >= 0 and ${table.healthMissing} >= 0 and ${table.healthChanged} >= 0`),
  index("artifact_runs_owner_started_index").on(table.ownerAccountId, table.startedAt, table.artifactId),
  index("artifact_runs_owner_producer_status_index").on(table.ownerAccountId, table.producerTool, table.status),
  index("artifact_runs_owner_health_index").on(table.ownerAccountId, table.health),
  index("artifact_runs_owner_series_index").on(table.ownerAccountId, table.seriesKey, table.startedAt, table.artifactId),
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

export const consoleAuthorizations = pgTable("console_authorizations", {
  id: text("id").primaryKey(),
  deviceCodeHash: text("device_code_hash").notNull(),
  userCode: text("user_code").notNull(),
  clientName: text("client_name").notNull(),
  clientType: text("client_type").notNull(),
  email: text("email"),
  otpHash: text("otp_hash"),
  otpAttempts: integer("otp_attempts").notNull(),
  emailSendCount: integer("email_send_count").notNull(),
  status: text("status").notNull(),
  approvedUserId: text("approved_user_id").references(() => consoleUsers.id, { onDelete: "restrict" }),
  lastPolledAt: timestamp("last_polled_at", { withTimezone: true }),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull(),
}, (table) => [
  check("console_authorizations_client_type_check", sql`${table.clientType} in ('cli', 'tv', 'agent', 'browser')`),
  check("console_authorizations_status_check", sql`${table.status} in ('pending', 'email_sent', 'approved', 'denied', 'consumed')`),
  check("console_authorizations_device_digest_check", sql`${table.deviceCodeHash} ~ '^[a-f0-9]{64}$'`),
  check("console_authorizations_otp_digest_check", sql`${table.otpHash} is null or ${table.otpHash} ~ '^[a-f0-9]{64}$'`),
  check("console_authorizations_attempts_check", sql`${table.otpAttempts} >= 0 and ${table.otpAttempts} <= 8`),
  check("console_authorizations_sends_check", sql`${table.emailSendCount} >= 0 and ${table.emailSendCount} <= 3`),
  check("console_authorizations_expiry_check", sql`${table.expiresAt} > ${table.createdAt}`),
  uniqueIndex("console_authorizations_device_digest_unique").on(table.deviceCodeHash),
  uniqueIndex("console_authorizations_user_code_unique").on(table.userCode),
  index("console_authorizations_expiry_index").on(table.expiresAt),
]);

export const consoleDeviceFamilies = pgTable("console_device_families", {
  id: text("id").primaryKey(),
  userId: text("user_id").notNull().references(() => consoleUsers.id, { onDelete: "cascade" }),
  clientName: text("client_name").notNull(),
  status: text("status").notNull(),
  absoluteExpiresAt: timestamp("absolute_expires_at", { withTimezone: true }).notNull(),
  idleExpiresAt: timestamp("idle_expires_at", { withTimezone: true }).notNull(),
  lastUsedAt: timestamp("last_used_at", { withTimezone: true }),
  revokedAt: timestamp("revoked_at", { withTimezone: true }),
  revokeReason: text("revoke_reason"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull(),
}, (table) => [
  check("console_device_families_status_check", sql`${table.status} in ('active', 'revoked')`),
  check("console_device_families_client_name_check", sql`length(${table.clientName}) between 1 and 80`),
  check("console_device_families_absolute_expiry_check", sql`${table.absoluteExpiresAt} > ${table.createdAt}`),
  check("console_device_families_idle_expiry_check", sql`${table.idleExpiresAt} > ${table.createdAt} and ${table.idleExpiresAt} <= ${table.absoluteExpiresAt}`),
  check("console_device_families_revocation_check", sql`(${table.status} = 'revoked') = (${table.revokedAt} is not null)`),
  index("console_device_families_user_index").on(table.userId, table.createdAt),
  index("console_device_families_expiry_index").on(table.status, table.idleExpiresAt, table.absoluteExpiresAt),
]);

export const consoleSessions = pgTable("console_sessions", {
  id: text("id").primaryKey(),
  userId: text("user_id").notNull().references(() => consoleUsers.id, { onDelete: "cascade" }),
  tokenHash: text("token_hash").notNull(),
  kind: text("kind").notNull(),
  refreshFamilyId: text("refresh_family_id").references(() => consoleDeviceFamilies.id, { onDelete: "cascade" }),
  lastFour: text("last_four").notNull(),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
  lastUsedAt: timestamp("last_used_at", { withTimezone: true }),
  revokedAt: timestamp("revoked_at", { withTimezone: true }),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull(),
}, (table) => [
  check("console_sessions_kind_check", sql`${table.kind} in ('web', 'device')`),
  check("console_sessions_token_digest_check", sql`${table.tokenHash} ~ '^[a-f0-9]{64}$'`),
  check("console_sessions_last_four_check", sql`length(${table.lastFour}) = 4`),
  check("console_sessions_expiry_check", sql`${table.expiresAt} > ${table.createdAt}`),
  check("console_sessions_web_family_check", sql`${table.kind} <> 'web' or ${table.refreshFamilyId} is null`),
  uniqueIndex("console_sessions_token_digest_unique").on(table.tokenHash),
  index("console_sessions_user_index").on(table.userId, table.createdAt),
  index("console_sessions_expiry_index").on(table.expiresAt),
  index("console_sessions_refresh_family_index").on(table.refreshFamilyId, table.createdAt),
]);

export const consoleRefreshTokens = pgTable("console_refresh_tokens", {
  id: text("id").primaryKey(),
  familyId: text("family_id").notNull().references(() => consoleDeviceFamilies.id, { onDelete: "cascade" }),
  generation: integer("generation").notNull(),
  tokenHash: text("token_hash").notNull(),
  lastFour: text("last_four").notNull(),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
  usedAt: timestamp("used_at", { withTimezone: true }),
  rotationId: text("rotation_id"),
  replacedByTokenHash: text("replaced_by_token_hash"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull(),
}, (table) => [
  check("console_refresh_tokens_generation_check", sql`${table.generation} >= 0`),
  check("console_refresh_tokens_digest_check", sql`${table.tokenHash} ~ '^[a-f0-9]{64}$'`),
  check("console_refresh_tokens_replacement_digest_check", sql`${table.replacedByTokenHash} is null or ${table.replacedByTokenHash} ~ '^[a-f0-9]{64}$'`),
  check("console_refresh_tokens_last_four_check", sql`length(${table.lastFour}) = 4`),
  check("console_refresh_tokens_expiry_check", sql`${table.expiresAt} > ${table.createdAt}`),
  check("console_refresh_tokens_rotation_check", sql`(${table.usedAt} is null and ${table.rotationId} is null and ${table.replacedByTokenHash} is null) or (${table.usedAt} is not null and ${table.rotationId} is not null and ${table.replacedByTokenHash} is not null)`),
  uniqueIndex("console_refresh_tokens_digest_unique").on(table.tokenHash),
  uniqueIndex("console_refresh_tokens_family_generation_unique").on(table.familyId, table.generation),
  uniqueIndex("console_refresh_tokens_family_rotation_unique").on(table.familyId, table.rotationId),
  index("console_refresh_tokens_expiry_index").on(table.expiresAt),
]);

export const consoleRateLimits = pgTable("console_rate_limits", {
  id: text("id").primaryKey(),
  action: text("action").notNull(),
  keyHash: text("key_hash").notNull(),
  windowStartedAt: timestamp("window_started_at", { withTimezone: true }).notNull(),
  count: integer("count").notNull(),
  expiresAt: timestamp("expires_at", { withTimezone: true }).notNull(),
}, (table) => [
  check("console_rate_limits_key_digest_check", sql`${table.keyHash} ~ '^[a-f0-9]{64}$'`),
  check("console_rate_limits_count_check", sql`${table.count} > 0`),
  check("console_rate_limits_expiry_check", sql`${table.expiresAt} > ${table.windowStartedAt}`),
  uniqueIndex("console_rate_limits_bucket_unique").on(table.action, table.keyHash, table.windowStartedAt),
  index("console_rate_limits_expiry_index").on(table.expiresAt),
]);
