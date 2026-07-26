import { z } from "zod";

import { artifactIdSchema, artifactKindSchema, producerSchema, sha256Schema } from "@/features/artifacts/contracts";
import { indexedEvidenceSchema, indexedHealthSchema, indexedOutcomeSchema, indexedRunSchema, indexedRunStatusSchema } from "@/features/runs/index-contract";

export const runCursorSchema = z.string().min(8).max(256).regex(/^[A-Za-z0-9_-]+$/u);

export const runListQuerySchema = z.object({
  after: runCursorSchema.optional(),
  from: z.string().datetime().optional(),
  health: indexedHealthSchema.shape.state.optional(),
  limit: z.number().int().min(1).max(100).default(50),
  producer: z.string().min(1).max(64).optional(),
  q: z.string().trim().min(1).max(160).optional(),
  status: indexedRunStatusSchema.optional(),
  to: z.string().datetime().optional(),
}).strict().refine((value) => !value.from || !value.to || Date.parse(value.to) >= Date.parse(value.from), {
  message: "to cannot precede from",
  path: ["to"],
});

export const runSummarySchema = z.object({
  artifactId: artifactIdSchema,
  counts: z.object({ artifacts: z.number().int().min(0), outcomes: z.number().int().min(0), steps: z.number().int().min(0) }).strict(),
  createdAt: z.string().datetime(),
  detector: z.object({ name: z.enum(["cairntrace-run", "glyphrun-run"]), version: z.string() }).strict(),
  evidence: z.array(indexedEvidenceSchema).max(200),
  health: indexedHealthSchema,
  outcomes: z.array(indexedOutcomeSchema).max(100),
  producer: producerSchema.required({ native_id: true, native_schema: true }),
  run: indexedRunSchema,
  runIndexSha256: sha256Schema,
  source: z.object({
    contentType: z.string(),
    kind: artifactKindSchema,
    sha256: sha256Schema,
    sizeBytes: z.number().int().positive(),
  }).strict(),
  updatedAt: z.string().datetime(),
}).strict();

export const runListResponseSchema = z.object({
  nextCursor: runCursorSchema.nullable(),
  runs: z.array(runSummarySchema).max(100),
  version: z.literal("filecheap-runs/1"),
}).strict();

export type RunStatus = z.infer<typeof indexedRunStatusSchema>;
export type RunHealth = z.infer<typeof indexedHealthSchema>["state"];
export type RunListQuery = z.infer<typeof runListQuerySchema>;
export type RunSummary = z.infer<typeof runSummarySchema>;
