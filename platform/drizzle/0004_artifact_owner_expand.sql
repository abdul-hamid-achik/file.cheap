ALTER TABLE "artifacts" ADD COLUMN "owner_account_id" text;--> statement-breakpoint
CREATE INDEX "artifacts_owner_created_index" ON "artifacts" USING btree ("owner_account_id","created_at","artifact_id");
