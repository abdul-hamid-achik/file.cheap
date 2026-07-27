-- Matches the console's effective run timestamp keyset. On a synthetic
-- 80k-row catalog this removes the full scan + sort from the first page.
CREATE INDEX "artifact_runs_owner_sort_index" ON "artifact_runs" USING btree ("owner_account_id",coalesce("started_at", "created_at"),"artifact_id");
