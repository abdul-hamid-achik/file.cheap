ALTER TABLE "artifacts" ADD COLUMN "plan_receipt_scheme" text;--> statement-breakpoint
ALTER TABLE "artifacts" ADD COLUMN "plan_receipt_kid" text;--> statement-breakpoint
ALTER TABLE "artifacts" ADD COLUMN "plan_receipt_nonce" text;--> statement-breakpoint
ALTER TABLE "artifacts" ADD COLUMN "plan_receipt_lookup" text;--> statement-breakpoint
CREATE UNIQUE INDEX "artifacts_plan_receipt_lookup_unique" ON "artifacts" USING btree ("plan_receipt_kid","plan_receipt_lookup") WHERE "artifacts"."plan_receipt_lookup" is not null;--> statement-breakpoint
ALTER TABLE "artifacts" ADD CONSTRAINT "artifacts_plan_receipt_shape_check" CHECK ((
      "artifacts"."plan_receipt_scheme" is null and
      "artifacts"."plan_receipt_kid" is null and
      "artifacts"."plan_receipt_nonce" is null and
      "artifacts"."plan_receipt_lookup" is null
    ) or (
      "artifacts"."plan_receipt_scheme" = 'hmac-sha256-v1' and
      "artifacts"."plan_receipt_kid" is not null and
      "artifacts"."plan_receipt_kid" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$' and
      "artifacts"."plan_receipt_nonce" is not null and
      "artifacts"."plan_receipt_nonce" ~ '^[A-Za-z0-9_-]{43}$' and
      "artifacts"."plan_receipt_lookup" is not null and
      "artifacts"."plan_receipt_lookup" ~ '^[A-Za-z0-9_-]{43}$'
    ) or (
      "artifacts"."plan_receipt_scheme" = 'legacy-random-hmac-sha256-v1' and
      "artifacts"."plan_receipt_kid" is not null and
      "artifacts"."plan_receipt_kid" ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$' and
      "artifacts"."plan_receipt_nonce" is null and
      "artifacts"."plan_receipt_lookup" is not null and
      "artifacts"."plan_receipt_lookup" ~ '^[A-Za-z0-9_-]{43}$'
    ));