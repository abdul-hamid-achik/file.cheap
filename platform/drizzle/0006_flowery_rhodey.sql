CREATE TABLE "console_device_families" (
	"id" text PRIMARY KEY NOT NULL,
	"user_id" text NOT NULL,
	"client_name" text NOT NULL,
	"status" text NOT NULL,
	"absolute_expires_at" timestamp with time zone NOT NULL,
	"idle_expires_at" timestamp with time zone NOT NULL,
	"last_used_at" timestamp with time zone,
	"revoked_at" timestamp with time zone,
	"revoke_reason" text,
	"created_at" timestamp with time zone NOT NULL,
	"updated_at" timestamp with time zone NOT NULL,
	CONSTRAINT "console_device_families_status_check" CHECK ("console_device_families"."status" in ('active', 'revoked')),
	CONSTRAINT "console_device_families_client_name_check" CHECK (length("console_device_families"."client_name") between 1 and 80),
	CONSTRAINT "console_device_families_absolute_expiry_check" CHECK ("console_device_families"."absolute_expires_at" > "console_device_families"."created_at"),
	CONSTRAINT "console_device_families_idle_expiry_check" CHECK ("console_device_families"."idle_expires_at" > "console_device_families"."created_at" and "console_device_families"."idle_expires_at" <= "console_device_families"."absolute_expires_at"),
	CONSTRAINT "console_device_families_revocation_check" CHECK (("console_device_families"."status" = 'revoked') = ("console_device_families"."revoked_at" is not null))
);
--> statement-breakpoint
CREATE TABLE "console_refresh_tokens" (
	"id" text PRIMARY KEY NOT NULL,
	"family_id" text NOT NULL,
	"generation" integer NOT NULL,
	"token_hash" text NOT NULL,
	"last_four" text NOT NULL,
	"expires_at" timestamp with time zone NOT NULL,
	"used_at" timestamp with time zone,
	"rotation_id" text,
	"replaced_by_token_hash" text,
	"created_at" timestamp with time zone NOT NULL,
	"updated_at" timestamp with time zone NOT NULL,
	CONSTRAINT "console_refresh_tokens_generation_check" CHECK ("console_refresh_tokens"."generation" >= 0),
	CONSTRAINT "console_refresh_tokens_digest_check" CHECK ("console_refresh_tokens"."token_hash" ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "console_refresh_tokens_replacement_digest_check" CHECK ("console_refresh_tokens"."replaced_by_token_hash" is null or "console_refresh_tokens"."replaced_by_token_hash" ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "console_refresh_tokens_last_four_check" CHECK (length("console_refresh_tokens"."last_four") = 4),
	CONSTRAINT "console_refresh_tokens_expiry_check" CHECK ("console_refresh_tokens"."expires_at" > "console_refresh_tokens"."created_at"),
	CONSTRAINT "console_refresh_tokens_rotation_check" CHECK (("console_refresh_tokens"."used_at" is null and "console_refresh_tokens"."rotation_id" is null and "console_refresh_tokens"."replaced_by_token_hash" is null) or ("console_refresh_tokens"."used_at" is not null and "console_refresh_tokens"."rotation_id" is not null and "console_refresh_tokens"."replaced_by_token_hash" is not null))
);
--> statement-breakpoint
ALTER TABLE "console_sessions" ADD COLUMN "refresh_family_id" text;--> statement-breakpoint
ALTER TABLE "console_device_families" ADD CONSTRAINT "console_device_families_user_id_console_users_id_fk" FOREIGN KEY ("user_id") REFERENCES "public"."console_users"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
ALTER TABLE "console_refresh_tokens" ADD CONSTRAINT "console_refresh_tokens_family_id_console_device_families_id_fk" FOREIGN KEY ("family_id") REFERENCES "public"."console_device_families"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "console_device_families_user_index" ON "console_device_families" USING btree ("user_id","created_at");--> statement-breakpoint
CREATE INDEX "console_device_families_expiry_index" ON "console_device_families" USING btree ("status","idle_expires_at","absolute_expires_at");--> statement-breakpoint
CREATE UNIQUE INDEX "console_refresh_tokens_digest_unique" ON "console_refresh_tokens" USING btree ("token_hash");--> statement-breakpoint
CREATE UNIQUE INDEX "console_refresh_tokens_family_generation_unique" ON "console_refresh_tokens" USING btree ("family_id","generation");--> statement-breakpoint
CREATE UNIQUE INDEX "console_refresh_tokens_family_rotation_unique" ON "console_refresh_tokens" USING btree ("family_id","rotation_id");--> statement-breakpoint
CREATE INDEX "console_refresh_tokens_expiry_index" ON "console_refresh_tokens" USING btree ("expires_at");--> statement-breakpoint
ALTER TABLE "console_sessions" ADD CONSTRAINT "console_sessions_refresh_family_id_console_device_families_id_fk" FOREIGN KEY ("refresh_family_id") REFERENCES "public"."console_device_families"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "console_sessions_refresh_family_index" ON "console_sessions" USING btree ("refresh_family_id","created_at");--> statement-breakpoint
ALTER TABLE "console_sessions" ADD CONSTRAINT "console_sessions_web_family_check" CHECK ("console_sessions"."kind" <> 'web' or "console_sessions"."refresh_family_id" is null);