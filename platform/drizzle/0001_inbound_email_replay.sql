CREATE TABLE "inbound_email_replays" (
	"id" text PRIMARY KEY NOT NULL,
	"svix_id_sha256" text NOT NULL,
	"email_id_sha256" text NOT NULL,
	"status" text NOT NULL,
	"attempts" integer NOT NULL,
	"lease_token" text,
	"processing_lease_expires_at" timestamp with time zone,
	"forwarded_at" timestamp with time zone,
	"ambiguous_at" timestamp with time zone,
	"expires_at" timestamp with time zone NOT NULL,
	"created_at" timestamp with time zone NOT NULL,
	"updated_at" timestamp with time zone NOT NULL,
	CONSTRAINT "inbound_email_replays_status_check" CHECK ("inbound_email_replays"."status" in ('processing', 'forwarded', 'ignored', 'ambiguous', 'rejected')),
	CONSTRAINT "inbound_email_replays_attempts_check" CHECK ("inbound_email_replays"."attempts" > 0),
	CONSTRAINT "inbound_email_replays_attempts_upper_bound_check" CHECK ("inbound_email_replays"."attempts" <= 8),
	CONSTRAINT "inbound_email_replays_svix_digest_check" CHECK ("inbound_email_replays"."svix_id_sha256" ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "inbound_email_replays_email_digest_check" CHECK ("inbound_email_replays"."email_id_sha256" ~ '^[a-f0-9]{64}$'),
	CONSTRAINT "inbound_email_replays_expiry_check" CHECK ("inbound_email_replays"."expires_at" > "inbound_email_replays"."created_at"),
	CONSTRAINT "inbound_email_replays_processing_lease_check" CHECK (("inbound_email_replays"."status" = 'processing') = ("inbound_email_replays"."lease_token" is not null and "inbound_email_replays"."processing_lease_expires_at" is not null)),
	CONSTRAINT "inbound_email_replays_forwarded_check" CHECK (("inbound_email_replays"."status" = 'forwarded') = ("inbound_email_replays"."forwarded_at" is not null)),
	CONSTRAINT "inbound_email_replays_ambiguous_check" CHECK (("inbound_email_replays"."status" = 'ambiguous') = ("inbound_email_replays"."ambiguous_at" is not null))
);
--> statement-breakpoint
CREATE UNIQUE INDEX "inbound_email_replays_svix_digest_unique" ON "inbound_email_replays" USING btree ("svix_id_sha256");--> statement-breakpoint
CREATE UNIQUE INDEX "inbound_email_replays_email_digest_unique" ON "inbound_email_replays" USING btree ("email_id_sha256");--> statement-breakpoint
CREATE INDEX "inbound_email_replays_expiry_index" ON "inbound_email_replays" USING btree ("expires_at");
