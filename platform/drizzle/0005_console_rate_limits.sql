CREATE TABLE "console_rate_limits" (
	"id" text PRIMARY KEY NOT NULL,
	"action" text NOT NULL,
	"key_hash" text NOT NULL,
	"window_started_at" timestamp with time zone NOT NULL,
	"count" integer NOT NULL,
	"expires_at" timestamp with time zone NOT NULL,
	CONSTRAINT "console_rate_limits_key_digest_check" CHECK ("console_rate_limits"."key_hash" ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "console_rate_limits_count_check" CHECK ("console_rate_limits"."count" > 0),
	CONSTRAINT "console_rate_limits_expiry_check" CHECK ("console_rate_limits"."expires_at" > "console_rate_limits"."window_started_at")
);
--> statement-breakpoint
CREATE UNIQUE INDEX "console_rate_limits_bucket_unique" ON "console_rate_limits" USING btree ("action","key_hash","window_started_at");--> statement-breakpoint
CREATE INDEX "console_rate_limits_expiry_index" ON "console_rate_limits" USING btree ("expires_at");
