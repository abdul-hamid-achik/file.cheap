import { sql } from "drizzle-orm";
import { check, index, integer, jsonb, pgTable, primaryKey, text, timestamp, uniqueIndex } from "drizzle-orm/pg-core";

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
  planReceiptScheme: text("plan_receipt_scheme"),
  planReceiptKid: text("plan_receipt_kid"),
  planReceiptNonce: text("plan_receipt_nonce"),
  planReceiptLookup: text("plan_receipt_lookup"),
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
  check(
    "artifacts_plan_receipt_shape_check",
    sql`(
      ${table.planReceiptScheme} is null and
      ${table.planReceiptKid} is null and
      ${table.planReceiptNonce} is null and
      ${table.planReceiptLookup} is null
    ) or (
      ${table.planReceiptScheme} = 'hmac-sha256-v1' and
      ${table.planReceiptKid} is not null and
      ${table.planReceiptKid} ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$' and
      ${table.planReceiptNonce} is not null and
      ${table.planReceiptNonce} ~ '^[A-Za-z0-9_-]{43}$' and
      ${table.planReceiptLookup} is not null and
      ${table.planReceiptLookup} ~ '^[A-Za-z0-9_-]{43}$'
    ) or (
      ${table.planReceiptScheme} = 'legacy-random-hmac-sha256-v1' and
      ${table.planReceiptKid} is not null and
      ${table.planReceiptKid} ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$' and
      ${table.planReceiptNonce} is null and
      ${table.planReceiptLookup} is not null and
      ${table.planReceiptLookup} ~ '^[A-Za-z0-9_-]{43}$'
    )`,
  ),
  uniqueIndex("artifacts_plan_token_unique").on(table.planToken),
  uniqueIndex("artifacts_plan_receipt_lookup_unique")
    .on(table.planReceiptKid, table.planReceiptLookup)
    .where(sql`${table.planReceiptLookup} is not null`),
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
  // Console run pagination orders by the effective start time so rows without
  // a producer timestamp remain in the same keyset. Keep this expression in
  // sync with DrizzleConsoleCatalogRepository.listRuns.
  index("artifact_runs_owner_sort_index").on(
    table.ownerAccountId,
    sql`coalesce(${table.startedAt}, ${table.createdAt})`,
    table.artifactId,
  ),
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

// Outbound verification is a recoverable state machine. The OTP itself is
// derived from keyed material and is never persisted; a stable authorization
// + delivery ordinal is enough to regenerate both the OTP and provider
// idempotency key after a timeout or process restart.
export const consoleVerificationDeliveries = pgTable("console_verification_deliveries", {
  authorizationId: text("authorization_id").notNull().references(
    () => consoleAuthorizations.id,
    { onDelete: "cascade" },
  ),
  deliveryNumber: integer("delivery_number").notNull(),
  email: text("email").notNull(),
  status: text("status").notNull(),
  leaseToken: text("lease_token"),
  leaseExpiresAt: timestamp("lease_expires_at", { withTimezone: true }),
  acceptedAt: timestamp("accepted_at", { withTimezone: true }),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull(),
  updatedAt: timestamp("updated_at", { withTimezone: true }).notNull(),
}, (table) => [
  primaryKey({
    columns: [table.authorizationId, table.deliveryNumber],
    name: "console_verification_deliveries_pk",
  }),
  check(
    "console_verification_deliveries_number_check",
    sql`${table.deliveryNumber} between 1 and 3`,
  ),
  check(
    "console_verification_deliveries_email_check",
    sql`length(${table.email}) between 3 and 320`,
  ),
  check(
    "console_verification_deliveries_status_check",
    sql`${table.status} in ('pending', 'sending', 'accepted')`,
  ),
  check(
    "console_verification_deliveries_lease_check",
    sql`(${table.status} = 'sending') = (${table.leaseToken} is not null and ${table.leaseExpiresAt} is not null)`,
  ),
  check(
    "console_verification_deliveries_acceptance_check",
    sql`(${table.status} = 'accepted') = (${table.acceptedAt} is not null)`,
  ),
  index("console_verification_deliveries_lease_index").on(
    table.status,
    table.leaseExpiresAt,
  ),
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

// Private operational state. One active row fences a retention worker; a
// stale row is terminally abandoned before a replacement can start. These
// records contain counts and allowlisted area names only, never artifact keys,
// paths, credentials, provider payloads, or raw errors.
export const privateRetentionRuns = pgTable("private_retention_runs", {
  id: text("id").primaryKey(),
  status: text("status").notNull(),
  startedAt: timestamp("started_at", { withTimezone: true }).notNull(),
  heartbeatAt: timestamp("heartbeat_at", { withTimezone: true }).notNull(),
  finishedAt: timestamp("finished_at", { withTimezone: true }),
  oldestDueAt: timestamp("oldest_due_at", { withTimezone: true }),
  failedAreas: text("failed_areas").array().notNull().default(sql`'{}'::text[]`),
  artifactCandidates: integer("artifact_candidates").notNull().default(0),
  artifactFailures: integer("artifact_failures").notNull().default(0),
  artifactsDeleted: integer("artifacts_deleted").notNull().default(0),
  inboundReplayRecordsDeleted: integer("inbound_replay_records_deleted").notNull().default(0),
  consoleAuthorizationRecordsDeleted: integer("console_authorization_records_deleted").notNull().default(0),
  consoleDeviceFamilyRecordsDeleted: integer("console_device_family_records_deleted").notNull().default(0),
  consoleSessionRecordsDeleted: integer("console_session_records_deleted").notNull().default(0),
  consoleRateLimitRecordsDeleted: integer("console_rate_limit_records_deleted").notNull().default(0),
  stagesAttempted: integer("stages_attempted").notNull().default(0),
  stagesSucceeded: integer("stages_succeeded").notNull().default(0),
  stagesFailed: integer("stages_failed").notNull().default(0),
}, (table) => [
  check(
    "private_retention_runs_id_check",
    sql`${table.id} ~ '^rtn_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'`,
  ),
  check(
    "private_retention_runs_status_check",
    sql`${table.status} in ('running', 'succeeded', 'partial', 'failed', 'abandoned')`,
  ),
  check(
    "private_retention_runs_terminal_check",
    sql`(${table.status} = 'running') = (${table.finishedAt} is null)`,
  ),
  check(
    "private_retention_runs_time_check",
    sql`${table.heartbeatAt} >= ${table.startedAt} and (${table.finishedAt} is null or (${table.finishedAt} >= ${table.heartbeatAt} and ${table.finishedAt} >= ${table.startedAt}))`,
  ),
  check(
    "private_retention_runs_failed_areas_check",
    sql`${table.failedAreas} <@ ARRAY['artifacts', 'inbound_email_replays', 'console_authorizations', 'console_device_families', 'console_sessions', 'console_rate_limits', 'backlog_probe', 'run_lease']::text[] and cardinality(${table.failedAreas}) <= 8`,
  ),
  check(
    "private_retention_runs_outcome_check",
    sql`(
      (${table.status} = 'running' and cardinality(${table.failedAreas}) = 0) or
      (${table.status} = 'succeeded' and cardinality(${table.failedAreas}) = 0) or
      (${table.status} in ('partial', 'failed') and cardinality(${table.failedAreas}) > 0) or
      (${table.status} = 'abandoned' and ${table.failedAreas} = ARRAY['run_lease']::text[])
    )`,
  ),
  check(
    "private_retention_runs_counters_check",
    sql`${table.artifactCandidates} >= 0 and ${table.artifactFailures} >= 0 and ${table.artifactsDeleted} >= 0 and ${table.inboundReplayRecordsDeleted} >= 0 and ${table.consoleAuthorizationRecordsDeleted} >= 0 and ${table.consoleDeviceFamilyRecordsDeleted} >= 0 and ${table.consoleSessionRecordsDeleted} >= 0 and ${table.consoleRateLimitRecordsDeleted} >= 0 and ${table.stagesAttempted} >= 0 and ${table.stagesSucceeded} >= 0 and ${table.stagesFailed} >= 0 and ${table.stagesAttempted} = ${table.stagesSucceeded} + ${table.stagesFailed}`,
  ),
  uniqueIndex("private_retention_runs_one_running_unique")
    .on(table.status)
    .where(sql`${table.status} = 'running'`),
  index("private_retention_runs_heartbeat_index").on(table.status, table.heartbeatAt),
  index("private_retention_runs_finished_index").on(table.finishedAt, table.id),
]);

// This ledger is append-only at both the domain port and database trigger. Its
// JSON object is intentionally narrow: each new event family must add an
// explicit schema branch instead of accepting free-form operational details.
export const privateActivityEvents = pgTable("private_activity_events", {
  id: text("id").primaryKey(),
  eventName: text("event_name").notNull(),
  actor: text("actor").notNull(),
  subjectType: text("subject_type").notNull(),
  subjectId: text("subject_id").notNull().references(
    () => privateRetentionRuns.id,
    { onDelete: "restrict" },
  ),
  details: jsonb("details").notNull(),
  recordedAt: timestamp("recorded_at", { withTimezone: true }).notNull(),
}, (table) => [
  check(
    "private_activity_events_id_check",
    sql`${table.id} ~ '^act_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'`,
  ),
  check(
    "private_activity_events_name_check",
    sql`${table.eventName} in ('private.retention_run.started', 'private.retention_run.succeeded', 'private.retention_run.partial', 'private.retention_run.failed', 'private.retention_run.abandoned')`,
  ),
  check("private_activity_events_actor_check", sql`${table.actor} = 'system:retention'`),
  check("private_activity_events_subject_check", sql`${table.subjectType} = 'retention_run'`),
  check(
    "private_activity_events_details_check",
    sql`jsonb_typeof(${table.details}) = 'object' and (
      (${table.eventName} = 'private.retention_run.started' and ${table.details} = '{}'::jsonb) or
      (
        ${table.eventName} <> 'private.retention_run.started' and
        ${table.details} ?& ARRAY['counters', 'failedAreas', 'oldestDueAt', 'status'] and
        (${table.details} - ARRAY['counters', 'failedAreas', 'oldestDueAt', 'status']::text[]) = '{}'::jsonb and
        jsonb_typeof(${table.details}->'counters') = 'object' and
        ${table.details}->'counters' ?& ARRAY['artifactCandidates', 'artifactFailures', 'artifactsDeleted', 'consoleAuthorizationRecordsDeleted', 'consoleDeviceFamilyRecordsDeleted', 'consoleRateLimitRecordsDeleted', 'consoleSessionRecordsDeleted', 'inboundReplayRecordsDeleted', 'stagesAttempted', 'stagesFailed', 'stagesSucceeded'] and
        ((${table.details}->'counters') - ARRAY['artifactCandidates', 'artifactFailures', 'artifactsDeleted', 'consoleAuthorizationRecordsDeleted', 'consoleDeviceFamilyRecordsDeleted', 'consoleRateLimitRecordsDeleted', 'consoleSessionRecordsDeleted', 'inboundReplayRecordsDeleted', 'stagesAttempted', 'stagesFailed', 'stagesSucceeded']::text[]) = '{}'::jsonb and
        jsonb_typeof(${table.details}#>'{counters,artifactCandidates}') = 'number' and (${table.details}#>>'{counters,artifactCandidates}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof(${table.details}#>'{counters,artifactFailures}') = 'number' and (${table.details}#>>'{counters,artifactFailures}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof(${table.details}#>'{counters,artifactsDeleted}') = 'number' and (${table.details}#>>'{counters,artifactsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof(${table.details}#>'{counters,consoleAuthorizationRecordsDeleted}') = 'number' and (${table.details}#>>'{counters,consoleAuthorizationRecordsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof(${table.details}#>'{counters,consoleDeviceFamilyRecordsDeleted}') = 'number' and (${table.details}#>>'{counters,consoleDeviceFamilyRecordsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof(${table.details}#>'{counters,consoleRateLimitRecordsDeleted}') = 'number' and (${table.details}#>>'{counters,consoleRateLimitRecordsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof(${table.details}#>'{counters,consoleSessionRecordsDeleted}') = 'number' and (${table.details}#>>'{counters,consoleSessionRecordsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof(${table.details}#>'{counters,inboundReplayRecordsDeleted}') = 'number' and (${table.details}#>>'{counters,inboundReplayRecordsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof(${table.details}#>'{counters,stagesAttempted}') = 'number' and (${table.details}#>>'{counters,stagesAttempted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof(${table.details}#>'{counters,stagesFailed}') = 'number' and (${table.details}#>>'{counters,stagesFailed}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof(${table.details}#>'{counters,stagesSucceeded}') = 'number' and (${table.details}#>>'{counters,stagesSucceeded}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof(${table.details}->'failedAreas') = 'array' and
        jsonb_array_length(${table.details}->'failedAreas') <= 8 and
        not jsonb_path_exists(${table.details}, '$.failedAreas[*] ? (@.type() != "string")') and
        not jsonb_path_exists(${table.details}, '$.failedAreas[*] ? (@ != "artifacts" && @ != "inbound_email_replays" && @ != "console_authorizations" && @ != "console_device_families" && @ != "console_sessions" && @ != "console_rate_limits" && @ != "backlog_probe" && @ != "run_lease")') and
        jsonb_typeof(${table.details}->'oldestDueAt') in ('null', 'string') and
        (${table.details}->'oldestDueAt' = 'null'::jsonb or (${table.details}->>'oldestDueAt') ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}[.][0-9]{3}Z$') and
        jsonb_typeof(${table.details}->'status') = 'string' and
        (${table.details}->>'status') = split_part(${table.eventName}, '.', 3) and
        (${table.details}->>'status') in ('succeeded', 'partial', 'failed', 'abandoned')
      )
    )`,
  ),
  index("private_activity_events_recorded_index").on(table.recordedAt, table.id),
  index("private_activity_events_subject_index").on(table.subjectType, table.subjectId, table.recordedAt),
]);
