CREATE TABLE "console_verification_deliveries" (
	"authorization_id" text NOT NULL,
	"delivery_number" integer NOT NULL,
	"email" text NOT NULL,
	"status" text NOT NULL,
	"lease_token" text,
	"lease_expires_at" timestamp with time zone,
	"accepted_at" timestamp with time zone,
	"created_at" timestamp with time zone NOT NULL,
	"updated_at" timestamp with time zone NOT NULL,
	CONSTRAINT "console_verification_deliveries_pk" PRIMARY KEY("authorization_id","delivery_number"),
	CONSTRAINT "console_verification_deliveries_number_check" CHECK ("console_verification_deliveries"."delivery_number" between 1 and 3),
	CONSTRAINT "console_verification_deliveries_email_check" CHECK (length("console_verification_deliveries"."email") between 3 and 320),
	CONSTRAINT "console_verification_deliveries_status_check" CHECK ("console_verification_deliveries"."status" in ('pending', 'sending', 'accepted')),
	CONSTRAINT "console_verification_deliveries_lease_check" CHECK (("console_verification_deliveries"."status" = 'sending') = ("console_verification_deliveries"."lease_token" is not null and "console_verification_deliveries"."lease_expires_at" is not null)),
	CONSTRAINT "console_verification_deliveries_acceptance_check" CHECK (("console_verification_deliveries"."status" = 'accepted') = ("console_verification_deliveries"."accepted_at" is not null))
);
--> statement-breakpoint
ALTER TABLE "console_verification_deliveries" ADD CONSTRAINT "console_verification_deliveries_authorization_id_console_authorizations_id_fk" FOREIGN KEY ("authorization_id") REFERENCES "public"."console_authorizations"("id") ON DELETE cascade ON UPDATE no action;--> statement-breakpoint
CREATE INDEX "console_verification_deliveries_lease_index" ON "console_verification_deliveries" USING btree ("status","lease_expires_at");