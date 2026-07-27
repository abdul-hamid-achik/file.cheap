import { z } from "zod";

import {
  artifactKindSchema,
  artifactSummarySchema,
  producerSchema,
} from "@/features/artifacts/contracts";
import {
  indexedHealthSchema,
  indexedRunStatusSchema,
} from "@/features/runs/index-contract";
import { runSummarySchema } from "@/features/runs/contracts";

export const consoleCatalogCursorSchema = z
  .string()
  .min(8)
  .max(320)
  .regex(/^[A-Za-z0-9_-]+$/u);

const pageDirectionSchema = z.enum(["next", "previous"]).default("next");
const pageLimitSchema = z.number().int().min(1).max(50).default(25);
const searchSchema = z.string().trim().min(1).max(160);

export const consoleArtifactListQuerySchema = z.object({
  cursor: consoleCatalogCursorSchema.optional(),
  direction: pageDirectionSchema,
  kind: artifactKindSchema.optional(),
  limit: pageLimitSchema,
  producer: producerSchema.shape.tool.optional(),
  q: searchSchema.optional(),
}).strict().refine((value) => value.direction !== "previous" || value.cursor, {
  message: "a cursor is required for previous-page traversal",
  path: ["cursor"],
});

export const consoleRunListQuerySchema = z.object({
  cursor: consoleCatalogCursorSchema.optional(),
  direction: pageDirectionSchema,
  from: z.string().datetime().optional(),
  health: indexedHealthSchema.shape.state.optional(),
  limit: pageLimitSchema,
  producer: producerSchema.shape.tool.optional(),
  q: searchSchema.optional(),
  status: indexedRunStatusSchema.optional(),
  to: z.string().datetime().optional(),
}).strict()
  .refine((value) => value.direction !== "previous" || value.cursor, {
    message: "a cursor is required for previous-page traversal",
    path: ["cursor"],
  })
  .refine((value) => !value.from || !value.to || Date.parse(value.to) >= Date.parse(value.from), {
    message: "to cannot precede from",
    path: ["to"],
  });

const pageInfoSchema = z.object({
  endCursor: consoleCatalogCursorSchema.nullable(),
  hasNextPage: z.boolean(),
  hasPreviousPage: z.boolean(),
  startCursor: consoleCatalogCursorSchema.nullable(),
}).strict();

const facetValueSchema = z.object({
  count: z.number().int().min(0),
  value: z.string().min(1).max(256),
}).strict();

export const consoleArtifactListResponseSchema = z.object({
  artifacts: z.array(artifactSummarySchema).max(50),
  facets: z.object({
    kinds: z.array(facetValueSchema),
    producers: z.array(facetValueSchema),
  }).strict(),
  filteredTotal: z.number().int().min(0),
  overview: z.object({
    expiringSoonCount: z.number().int().min(0),
    recordedCount: z.number().int().min(0),
    totalBytes: z.number().int().min(0),
    transferableCount: z.number().int().min(0),
    verifiedCount: z.number().int().min(0),
  }).strict(),
  pageInfo: pageInfoSchema,
  version: z.literal("filecheap-console-artifacts/1"),
}).strict();

export const consoleRunListResponseSchema = z.object({
  facets: z.object({
    health: z.array(facetValueSchema).max(16),
    producers: z.array(facetValueSchema),
    statuses: z.array(facetValueSchema).max(16),
  }).strict(),
  filteredTotal: z.number().int().min(0),
  overview: z.object({
    activeCount: z.number().int().min(0),
    healthyCount: z.number().int().min(0),
    indexedEvidenceCount: z.number().int().min(0),
    passedCount: z.number().int().min(0),
    recordedCount: z.number().int().min(0),
  }).strict(),
  pageInfo: pageInfoSchema,
  runs: z.array(runSummarySchema).max(50),
  version: z.literal("filecheap-console-runs/1"),
}).strict();

export type ConsoleArtifactListQuery = z.infer<typeof consoleArtifactListQuerySchema>;
export type ConsoleArtifactListResponse = z.infer<typeof consoleArtifactListResponseSchema>;
export type ConsoleRunListQuery = z.infer<typeof consoleRunListQuerySchema>;
export type ConsoleRunListResponse = z.infer<typeof consoleRunListResponseSchema>;
export type ConsoleCatalogPageInfo = z.infer<typeof pageInfoSchema>;
export type ConsoleCatalogFacetValue = z.infer<typeof facetValueSchema>;
