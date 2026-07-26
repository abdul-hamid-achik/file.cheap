-- Expand-only: the replacement CHECK accepts a strict superset of the old one
-- (2097152 -> 67108864 bytes), so no existing row can violate it and the
-- previous deployment keeps working against this schema. Per-producer quotas
-- are enforced at runtime; this constraint is only the global ceiling.
ALTER TABLE "artifact_objects" DROP CONSTRAINT "artifact_objects_size_check";--> statement-breakpoint
ALTER TABLE "artifacts" DROP CONSTRAINT "artifacts_size_check";--> statement-breakpoint
ALTER TABLE "artifact_objects" ADD CONSTRAINT "artifact_objects_size_check" CHECK ("artifact_objects"."size_bytes" > 0 and "artifact_objects"."size_bytes" <= 67108864);--> statement-breakpoint
ALTER TABLE "artifacts" ADD CONSTRAINT "artifacts_size_check" CHECK ("artifacts"."size_bytes" > 0 and "artifacts"."size_bytes" <= 67108864);