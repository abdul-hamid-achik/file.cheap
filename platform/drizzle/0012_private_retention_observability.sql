CREATE TABLE "private_activity_events" (
	"id" text PRIMARY KEY NOT NULL,
	"event_name" text NOT NULL,
	"actor" text NOT NULL,
	"subject_type" text NOT NULL,
	"subject_id" text NOT NULL,
	"details" jsonb NOT NULL,
	"recorded_at" timestamp with time zone NOT NULL,
	CONSTRAINT "private_activity_events_id_check" CHECK ("private_activity_events"."id" ~ '^act_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'),
	CONSTRAINT "private_activity_events_name_check" CHECK ("private_activity_events"."event_name" in ('private.retention_run.started', 'private.retention_run.succeeded', 'private.retention_run.partial', 'private.retention_run.failed', 'private.retention_run.abandoned')),
	CONSTRAINT "private_activity_events_actor_check" CHECK ("private_activity_events"."actor" = 'system:retention'),
	CONSTRAINT "private_activity_events_subject_check" CHECK ("private_activity_events"."subject_type" = 'retention_run'),
	CONSTRAINT "private_activity_events_details_check" CHECK (jsonb_typeof("private_activity_events"."details") = 'object' and (
      ("private_activity_events"."event_name" = 'private.retention_run.started' and "private_activity_events"."details" = '{}'::jsonb) or
      (
        "private_activity_events"."event_name" <> 'private.retention_run.started' and
        "private_activity_events"."details" ?& ARRAY['counters', 'failedAreas', 'oldestDueAt', 'status'] and
        ("private_activity_events"."details" - ARRAY['counters', 'failedAreas', 'oldestDueAt', 'status']::text[]) = '{}'::jsonb and
        jsonb_typeof("private_activity_events"."details"->'counters') = 'object' and
        "private_activity_events"."details"->'counters' ?& ARRAY['artifactCandidates', 'artifactFailures', 'artifactsDeleted', 'consoleAuthorizationRecordsDeleted', 'consoleDeviceFamilyRecordsDeleted', 'consoleRateLimitRecordsDeleted', 'consoleSessionRecordsDeleted', 'inboundReplayRecordsDeleted', 'stagesAttempted', 'stagesFailed', 'stagesSucceeded'] and
        (("private_activity_events"."details"->'counters') - ARRAY['artifactCandidates', 'artifactFailures', 'artifactsDeleted', 'consoleAuthorizationRecordsDeleted', 'consoleDeviceFamilyRecordsDeleted', 'consoleRateLimitRecordsDeleted', 'consoleSessionRecordsDeleted', 'inboundReplayRecordsDeleted', 'stagesAttempted', 'stagesFailed', 'stagesSucceeded']::text[]) = '{}'::jsonb and
        jsonb_typeof("private_activity_events"."details"#>'{counters,artifactCandidates}') = 'number' and ("private_activity_events"."details"#>>'{counters,artifactCandidates}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof("private_activity_events"."details"#>'{counters,artifactFailures}') = 'number' and ("private_activity_events"."details"#>>'{counters,artifactFailures}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof("private_activity_events"."details"#>'{counters,artifactsDeleted}') = 'number' and ("private_activity_events"."details"#>>'{counters,artifactsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof("private_activity_events"."details"#>'{counters,consoleAuthorizationRecordsDeleted}') = 'number' and ("private_activity_events"."details"#>>'{counters,consoleAuthorizationRecordsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof("private_activity_events"."details"#>'{counters,consoleDeviceFamilyRecordsDeleted}') = 'number' and ("private_activity_events"."details"#>>'{counters,consoleDeviceFamilyRecordsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof("private_activity_events"."details"#>'{counters,consoleRateLimitRecordsDeleted}') = 'number' and ("private_activity_events"."details"#>>'{counters,consoleRateLimitRecordsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof("private_activity_events"."details"#>'{counters,consoleSessionRecordsDeleted}') = 'number' and ("private_activity_events"."details"#>>'{counters,consoleSessionRecordsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof("private_activity_events"."details"#>'{counters,inboundReplayRecordsDeleted}') = 'number' and ("private_activity_events"."details"#>>'{counters,inboundReplayRecordsDeleted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof("private_activity_events"."details"#>'{counters,stagesAttempted}') = 'number' and ("private_activity_events"."details"#>>'{counters,stagesAttempted}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof("private_activity_events"."details"#>'{counters,stagesFailed}') = 'number' and ("private_activity_events"."details"#>>'{counters,stagesFailed}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof("private_activity_events"."details"#>'{counters,stagesSucceeded}') = 'number' and ("private_activity_events"."details"#>>'{counters,stagesSucceeded}') ~ '^(0|[1-9][0-9]*)$' and
        jsonb_typeof("private_activity_events"."details"->'failedAreas') = 'array' and
        jsonb_array_length("private_activity_events"."details"->'failedAreas') <= 8 and
        not jsonb_path_exists("private_activity_events"."details", '$.failedAreas[*] ? (@.type() != "string")') and
        not jsonb_path_exists("private_activity_events"."details", '$.failedAreas[*] ? (@ != "artifacts" && @ != "inbound_email_replays" && @ != "console_authorizations" && @ != "console_device_families" && @ != "console_sessions" && @ != "console_rate_limits" && @ != "backlog_probe" && @ != "run_lease")') and
        jsonb_typeof("private_activity_events"."details"->'oldestDueAt') in ('null', 'string') and
        ("private_activity_events"."details"->'oldestDueAt' = 'null'::jsonb or ("private_activity_events"."details"->>'oldestDueAt') ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}[.][0-9]{3}Z$') and
        jsonb_typeof("private_activity_events"."details"->'status') = 'string' and
        ("private_activity_events"."details"->>'status') = split_part("private_activity_events"."event_name", '.', 3) and
        ("private_activity_events"."details"->>'status') in ('succeeded', 'partial', 'failed', 'abandoned')
      )
    ))
);
--> statement-breakpoint
CREATE TABLE "private_retention_runs" (
	"id" text PRIMARY KEY NOT NULL,
	"status" text NOT NULL,
	"started_at" timestamp with time zone NOT NULL,
	"heartbeat_at" timestamp with time zone NOT NULL,
	"finished_at" timestamp with time zone,
	"oldest_due_at" timestamp with time zone,
	"failed_areas" text[] DEFAULT '{}'::text[] NOT NULL,
	"artifact_candidates" integer DEFAULT 0 NOT NULL,
	"artifact_failures" integer DEFAULT 0 NOT NULL,
	"artifacts_deleted" integer DEFAULT 0 NOT NULL,
	"inbound_replay_records_deleted" integer DEFAULT 0 NOT NULL,
	"console_authorization_records_deleted" integer DEFAULT 0 NOT NULL,
	"console_device_family_records_deleted" integer DEFAULT 0 NOT NULL,
	"console_session_records_deleted" integer DEFAULT 0 NOT NULL,
	"console_rate_limit_records_deleted" integer DEFAULT 0 NOT NULL,
	"stages_attempted" integer DEFAULT 0 NOT NULL,
	"stages_succeeded" integer DEFAULT 0 NOT NULL,
	"stages_failed" integer DEFAULT 0 NOT NULL,
	CONSTRAINT "private_retention_runs_id_check" CHECK ("private_retention_runs"."id" ~ '^rtn_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'),
	CONSTRAINT "private_retention_runs_status_check" CHECK ("private_retention_runs"."status" in ('running', 'succeeded', 'partial', 'failed', 'abandoned')),
	CONSTRAINT "private_retention_runs_terminal_check" CHECK (("private_retention_runs"."status" = 'running') = ("private_retention_runs"."finished_at" is null)),
	CONSTRAINT "private_retention_runs_time_check" CHECK ("private_retention_runs"."heartbeat_at" >= "private_retention_runs"."started_at" and ("private_retention_runs"."finished_at" is null or ("private_retention_runs"."finished_at" >= "private_retention_runs"."heartbeat_at" and "private_retention_runs"."finished_at" >= "private_retention_runs"."started_at"))),
	CONSTRAINT "private_retention_runs_failed_areas_check" CHECK ("private_retention_runs"."failed_areas" <@ ARRAY['artifacts', 'inbound_email_replays', 'console_authorizations', 'console_device_families', 'console_sessions', 'console_rate_limits', 'backlog_probe', 'run_lease']::text[] and cardinality("private_retention_runs"."failed_areas") <= 8),
	CONSTRAINT "private_retention_runs_outcome_check" CHECK ((
      ("private_retention_runs"."status" = 'running' and cardinality("private_retention_runs"."failed_areas") = 0) or
      ("private_retention_runs"."status" = 'succeeded' and cardinality("private_retention_runs"."failed_areas") = 0) or
      ("private_retention_runs"."status" in ('partial', 'failed') and cardinality("private_retention_runs"."failed_areas") > 0) or
      ("private_retention_runs"."status" = 'abandoned' and "private_retention_runs"."failed_areas" = ARRAY['run_lease']::text[])
    )),
	CONSTRAINT "private_retention_runs_counters_check" CHECK ("private_retention_runs"."artifact_candidates" >= 0 and "private_retention_runs"."artifact_failures" >= 0 and "private_retention_runs"."artifacts_deleted" >= 0 and "private_retention_runs"."inbound_replay_records_deleted" >= 0 and "private_retention_runs"."console_authorization_records_deleted" >= 0 and "private_retention_runs"."console_device_family_records_deleted" >= 0 and "private_retention_runs"."console_session_records_deleted" >= 0 and "private_retention_runs"."console_rate_limit_records_deleted" >= 0 and "private_retention_runs"."stages_attempted" >= 0 and "private_retention_runs"."stages_succeeded" >= 0 and "private_retention_runs"."stages_failed" >= 0 and "private_retention_runs"."stages_attempted" = "private_retention_runs"."stages_succeeded" + "private_retention_runs"."stages_failed")
);
--> statement-breakpoint
ALTER TABLE "private_activity_events" ADD CONSTRAINT "private_activity_events_subject_id_private_retention_runs_id_fk" FOREIGN KEY ("subject_id") REFERENCES "public"."private_retention_runs"("id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "private_activity_events_recorded_index" ON "private_activity_events" USING btree ("recorded_at","id");--> statement-breakpoint
CREATE INDEX "private_activity_events_subject_index" ON "private_activity_events" USING btree ("subject_type","subject_id","recorded_at");--> statement-breakpoint
CREATE UNIQUE INDEX "private_retention_runs_one_running_unique" ON "private_retention_runs" USING btree ("status") WHERE "private_retention_runs"."status" = 'running';--> statement-breakpoint
CREATE INDEX "private_retention_runs_heartbeat_index" ON "private_retention_runs" USING btree ("status","heartbeat_at");--> statement-breakpoint
CREATE INDEX "private_retention_runs_finished_index" ON "private_retention_runs" USING btree ("finished_at","id");--> statement-breakpoint
CREATE FUNCTION "private_activity_events_reject_mutation"() RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	RAISE EXCEPTION 'private activity events are append-only' USING ERRCODE = '55000';
END;
$$;--> statement-breakpoint
CREATE TRIGGER "private_activity_events_append_only"
BEFORE UPDATE OR DELETE ON "private_activity_events"
FOR EACH ROW EXECUTE FUNCTION "private_activity_events_reject_mutation"();
