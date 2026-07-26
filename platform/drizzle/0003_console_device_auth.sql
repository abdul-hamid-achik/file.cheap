CREATE TABLE "console_authorizations" (
	"id" text PRIMARY KEY NOT NULL,
	"device_code_hash" text NOT NULL,
	"user_code" text NOT NULL,
	"client_name" text NOT NULL,
	"client_type" text NOT NULL,
	"email" text,
	"otp_hash" text,
	"otp_attempts" integer NOT NULL,
	"email_send_count" integer NOT NULL,
	"status" text NOT NULL,
	"approved_user_id" text,
	"last_polled_at" timestamp with time zone,
	"expires_at" timestamp with time zone NOT NULL,
	"created_at" timestamp with time zone NOT NULL,
	"updated_at" timestamp with time zone NOT NULL,
	CONSTRAINT "console_authorizations_client_type_check" CHECK ("console_authorizations"."client_type" in ('cli', 'tv', 'agent', 'browser')),
	CONSTRAINT "console_authorizations_status_check" CHECK ("console_authorizations"."status" in ('pending', 'email_sent', 'approved', 'denied', 'consumed')),
	CONSTRAINT "console_authorizations_device_digest_check" CHECK ("console_authorizations"."device_code_hash" ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "console_authorizations_otp_digest_check" CHECK ("console_authorizations"."otp_hash" is null or "console_authorizations"."otp_hash" ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "console_authorizations_attempts_check" CHECK ("console_authorizations"."otp_attempts" >= 0 and "console_authorizations"."otp_attempts" <= 8),
	CONSTRAINT "console_authorizations_sends_check" CHECK ("console_authorizations"."email_send_count" >= 0 and "console_authorizations"."email_send_count" <= 3),
	CONSTRAINT "console_authorizations_expiry_check" CHECK ("console_authorizations"."expires_at" > "console_authorizations"."created_at")
);
--> statement-breakpoint
CREATE TABLE "console_sessions" (
	"id" text PRIMARY KEY NOT NULL,
	"user_id" text NOT NULL,
	"token_hash" text NOT NULL,
	"kind" text NOT NULL,
	"last_four" text NOT NULL,
	"expires_at" timestamp with time zone NOT NULL,
	"last_used_at" timestamp with time zone,
	"revoked_at" timestamp with time zone,
	"created_at" timestamp with time zone NOT NULL,
	"updated_at" timestamp with time zone NOT NULL,
	CONSTRAINT "console_sessions_kind_check" CHECK ("console_sessions"."kind" in ('web', 'device')),
	CONSTRAINT "console_sessions_token_digest_check" CHECK ("console_sessions"."token_hash" ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "console_sessions_last_four_check" CHECK (length("console_sessions"."last_four") = 4),
	CONSTRAINT "console_sessions_expiry_check" CHECK ("console_sessions"."expires_at" > "console_sessions"."created_at")
);
--> statement-breakpoint
CREATE TABLE "console_users" (
	"id" text PRIMARY KEY NOT NULL,
	"email" text NOT NULL,
	"created_at" timestamp with time zone NOT NULL,
	"updated_at" timestamp with time zone NOT NULL
);
--> statement-breakpoint
ALTER TABLE "console_authorizations" ADD CONSTRAINT "console_authorizations_approved_user_id_console_users_id_fk" FOREIGN KEY ("approved_user_id") REFERENCES "public"."console_users"("id") ON DELETE restrict ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "console_sessions" ADD CONSTRAINT "console_sessions_user_id_console_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."console_users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE UNIQUE INDEX "console_authorizations_device_digest_unique" ON "console_authorizations" USING btree ("device_code_hash");--> statement-breakpoint
CREATE UNIQUE INDEX "console_authorizations_user_code_unique" ON "console_authorizations" USING btree ("user_code");--> statement-breakpoint
CREATE INDEX "console_authorizations_expiry_index" ON "console_authorizations" USING btree ("expires_at");--> statement-breakpoint
CREATE UNIQUE INDEX "console_sessions_token_digest_unique" ON "console_sessions" USING btree ("token_hash");--> statement-breakpoint
CREATE INDEX "console_sessions_user_index" ON "console_sessions" USING btree ("user_id","created_at");--> statement-breakpoint
CREATE INDEX "console_sessions_expiry_index" ON "console_sessions" USING btree ("expires_at");--> statement-breakpoint
CREATE UNIQUE INDEX "console_users_email_unique" ON "console_users" USING btree ("email");
