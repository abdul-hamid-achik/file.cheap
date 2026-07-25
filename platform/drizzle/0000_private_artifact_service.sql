CREATE TABLE "artifact_objects" (
	"id" text PRIMARY KEY NOT NULL,
	"artifact_id" text NOT NULL,
	"ordinal" integer NOT NULL,
	"object_key" text NOT NULL,
	"sha256" text NOT NULL,
	"size_bytes" integer NOT NULL,
	"content_type" text NOT NULL,
	"etag" text,
	"created_at" timestamp with time zone NOT NULL,
	CONSTRAINT "artifact_objects_ordinal_check" CHECK ("artifact_objects"."ordinal" >= 0),
	CONSTRAINT "artifact_objects_size_check" CHECK ("artifact_objects"."size_bytes" > 0 and "artifact_objects"."size_bytes" <= 2097152)
);
--> statement-breakpoint
CREATE TABLE "artifacts" (
	"artifact_id" text PRIMARY KEY NOT NULL,
	"kind" text NOT NULL,
	"producer" jsonb NOT NULL,
	"sha256" text NOT NULL,
	"size_bytes" integer NOT NULL,
	"content_type" text NOT NULL,
	"state" text NOT NULL,
	"verification" text NOT NULL,
	"plan_token" text NOT NULL,
	"plan_expires_at" timestamp with time zone NOT NULL,
	"expires_at" timestamp with time zone,
	"committed_at" timestamp with time zone,
	"deleting_at" timestamp with time zone,
	"deleted_at" timestamp with time zone,
	"created_at" timestamp with time zone NOT NULL,
	CONSTRAINT "artifacts_state_check" CHECK ("artifacts"."state" in ('planned', 'committed', 'deleting', 'deleted')),
	CONSTRAINT "artifacts_verification_check" CHECK ("artifacts"."verification" in ('server-sha256')),
	CONSTRAINT "artifacts_sha256_check" CHECK ("artifacts"."sha256" ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "artifacts_size_check" CHECK ("artifacts"."size_bytes" > 0 and "artifacts"."size_bytes" <= 2097152),
	CONSTRAINT "artifacts_expiry_check" CHECK ("artifacts"."expires_at" is null or "artifacts"."expires_at" > "artifacts"."created_at")
);
--> statement-breakpoint
ALTER TABLE "artifact_objects" ADD CONSTRAINT "artifact_objects_artifact_id_artifacts_artifact_id_fk" FOREIGN KEY ("artifact_id") REFERENCES "public"."artifacts"("artifact_id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "artifact_objects_key_unique" ON "artifact_objects" USING btree ("object_key");--> statement-breakpoint
CREATE UNIQUE INDEX "artifact_objects_artifact_ordinal_unique" ON "artifact_objects" USING btree ("artifact_id","ordinal");--> statement-breakpoint
CREATE UNIQUE INDEX "artifacts_plan_token_unique" ON "artifacts" USING btree ("plan_token");--> statement-breakpoint
CREATE INDEX "artifacts_retention_index" ON "artifacts" USING btree ("state","expires_at");