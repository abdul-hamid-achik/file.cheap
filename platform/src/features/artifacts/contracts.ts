import { z } from "zod";

import { maximumArtifactBytes } from "@/shared/config/limits";
import { runIndexSchema } from "@/features/runs/index-contract";

export const artifactIdSchema = z.string().regex(/^art_[A-Za-z0-9_-]{16,96}$/);
export const sha256Schema = z.string().regex(/^[a-f0-9]{64}$/);
export const artifactKindSchema = z.string().max(128).regex(/^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$/);
const contentTypeSchema = z
  .string()
  .min(3)
  .max(255)
  .regex(
    /^[A-Za-z0-9!#$&^_.+-]+\/[A-Za-z0-9!#$&^_.+-]+$/,
    "must be a media type without parameters",
  );
const nativeSchemaSchema = z
  .string()
  .max(256)
  .refine(isSafeNativeSchema, {
    message: "must be a credential-free URN or HTTPS URI without a query",
  });
const entrypointSchema = z
  .string()
  .min(1)
  .max(512)
  .regex(
    /^[A-Za-z0-9._-]+(?:\/[A-Za-z0-9._-]+)*$/,
    "must be a safe slash-separated relative path",
  )
  .refine(
    (value) => value.split("/").every((segment) => segment !== "." && segment !== ".."),
    "must not contain dot path segments",
  );
export const producerSchema = z.object({
  tool: z.string().min(1).max(64).regex(/^[A-Za-z0-9][A-Za-z0-9._-]*$/),
  version: z.string().min(1).max(64).regex(/^[A-Za-z0-9][A-Za-z0-9._+\-]*$/).optional(),
  native_schema: nativeSchemaSchema.optional(),
  native_id: z.string().min(1).max(160).regex(/^[A-Za-z0-9][A-Za-z0-9._:-]*$/).optional(),
  entrypoint: entrypointSchema.optional(),
}).strict();

export const artifactPlanInputSchema = z.object({
  contentType: contentTypeSchema,
  expiresAt: z.string().datetime().refine((value) => {
    const milliseconds = Date.parse(value);
    return milliseconds > Date.now() && milliseconds <= Date.now() + 31 * 24 * 60 * 60 * 1000;
  }, "must be between now and 31 days from now").optional(),
  idempotencyKey: z.string().uuid().transform((value) => value.toLowerCase()),
  kind: artifactKindSchema,
  producer: producerSchema,
  runIndex: runIndexSchema.optional(),
  sha256: sha256Schema,
  sizeBytes: z.number().int().positive().max(maximumArtifactBytes),
}).strict().superRefine((value, context) => {
  if (!value.runIndex) return;
  if (!value.producer.native_schema) {
    context.addIssue({ code: "custom", message: "is required for an indexed run", path: ["producer", "native_schema"] });
  }
  if (value.producer.native_id !== value.runIndex.run.nativeId) {
    context.addIssue({ code: "custom", message: "must match runIndex.run.nativeId", path: ["producer", "native_id"] });
  }
  const expectedDetector = value.producer.tool === "cairntrace"
    ? "cairntrace-run"
    : value.producer.tool === "glyphrun"
      ? "glyphrun-run"
      : null;
  if (!expectedDetector || value.runIndex.detector.name !== expectedDetector) {
    context.addIssue({ code: "custom", message: "must match the authenticated producer detector", path: ["runIndex", "detector", "name"] });
  }
});

export const artifactCommitInputSchema = z.object({
  receipt: z.string().uuid(),
}).strict();

export const artifactDownloadInputSchema = z.object({
  artifactId: artifactIdSchema,
}).strict();

export const artifactListQuerySchema = z.object({
  after: artifactIdSchema.optional(),
  limit: z.number().int().min(1).max(100).default(50),
}).strict();

export const artifactSummarySchema = z.object({
  artifact: z.object({
    artifactId: artifactIdSchema,
    committedAt: z.string().datetime().nullable(),
    contentType: z.string(),
    expiresAt: z.string().datetime().nullable(),
    kind: artifactKindSchema,
    producer: producerSchema,
    sha256: sha256Schema,
    sizeBytes: z.number().int().positive(),
    state: z.enum(["planned", "committed", "deleting", "deleted"]),
    verification: z.literal("server-sha256"),
  }).strict(),
  artifactRef: z.object({
    $schema: z.literal("urn:filecheap.dev:artifact-ref:v1"),
    artifact_id: artifactIdSchema,
    kind: artifactKindSchema,
    producer: producerSchema,
    provider: z.literal("fcheap-cloud"),
    uri: z.string().regex(/^fcheap:\/\/cloud\/vaults\/private\/artifacts\//),
    version: z.literal(1),
  }).strict(),
}).strict();

export const artifactPlanResponseSchema = artifactSummarySchema.extend({
  artifact: artifactSummarySchema.shape.artifact.extend({
    committedAt: z.null(),
    state: z.literal("planned"),
  }).strict(),
  receipt: z.string().uuid(),
  upload: z.object({
    expiresAt: z.string().datetime(),
    headers: z.record(z.string(), z.string()),
    method: z.literal("PUT"),
    url: z.string().url(),
  }).strict(),
}).strict();

export const artifactPlanReplayResponseSchema = artifactSummarySchema.extend({
  artifact: artifactSummarySchema.shape.artifact.extend({
    committedAt: z.string().datetime(),
    state: z.literal("committed"),
  }).strict(),
}).strict();

export const artifactPlanResultSchema = z.union([
  artifactPlanResponseSchema,
  artifactPlanReplayResponseSchema,
]);

export const artifactDownloadResponseSchema = artifactSummarySchema.extend({
  download: z.object({
    expiresAt: z.string().datetime(),
    headers: z.record(z.string(), z.string()),
    method: z.literal("GET"),
    url: z.string().url(),
  }).strict(),
}).strict();

export const artifactListResponseSchema = z.object({
  artifacts: z.array(artifactSummarySchema).max(100),
  nextCursor: artifactIdSchema.nullable(),
  version: z.literal("filecheap-artifacts/1"),
}).strict();

export type ArtifactPlanInput = z.infer<typeof artifactPlanInputSchema>;
export type ArtifactCommitInput = z.infer<typeof artifactCommitInputSchema>;
export type ArtifactDownloadInput = z.infer<typeof artifactDownloadInputSchema>;
export type ArtifactSummary = z.infer<typeof artifactSummarySchema>;
export type ArtifactPlanResponse = z.infer<typeof artifactPlanResponseSchema>;
export type ArtifactPlanReplayResponse = z.infer<typeof artifactPlanReplayResponseSchema>;
export type ArtifactPlanResult = z.infer<typeof artifactPlanResultSchema>;
export type ArtifactDownloadResponse = z.infer<typeof artifactDownloadResponseSchema>;
export type ArtifactListQuery = z.infer<typeof artifactListQuerySchema>;

function isSafeNativeSchema(value: string): boolean {
  if (!/^[\x21-\x7e]+$/.test(value) || value.includes("?")) {
    return false;
  }
  if (value.startsWith("urn:")) {
    return value.length > "urn:".length;
  }
  try {
    const parsed = new URL(value);
    return (
      parsed.protocol === "https:" &&
      parsed.hostname.length > 0 &&
      parsed.username === "" &&
      parsed.password === "" &&
      parsed.search === ""
    );
  } catch {
    return false;
  }
}
