import { z } from "zod";

export const stashContentType = "application/vnd.filecheap.stash" as const;
export const protocolV1MaxObjectBytes = 64 * 1024 * 1024;
export const protocolV1MaximumCatalogEntries = 1_000;

export const protocolV1DateTimeSchema = z
  .string()
  .min(20)
  .max(40)
  .pipe(z.iso.datetime());

export const protocolV1ObjectKeySchema = z.string().min(1).max(1_024);

export const stashIdSchema = z
  .string()
  .min(1)
  .max(128)
  .regex(/^[A-Za-z0-9][A-Za-z0-9._-]*$/, {
    message: "must contain only letters, numbers, dots, underscores, and hyphens",
  });

export const sha256Schema = z
  .string()
  .regex(/^[a-f0-9]{64}$/, { message: "must be a lowercase SHA-256 hex digest" });

export const createPlanSchema = z
  .object({
    contentType: z.literal(stashContentType).default(stashContentType),
    sha256: sha256Schema,
    sizeBytes: z.number().int().positive().max(protocolV1MaxObjectBytes),
    stashId: stashIdSchema,
  })
  .strict();

export const commitPlanSchema = z
  .object({
    receipt: z.string().min(1).max(4_096),
  })
  .strict();

export const createDownloadSchema = z
  .object({
    stashId: stashIdSchema,
  })
  .strict();

export const transferGrantSchema = z
  .object({
    expiresAt: protocolV1DateTimeSchema,
    headers: z
      .record(z.string().min(1).max(128), z.string().max(4_096))
      .refine((headers) => Object.keys(headers).length <= 16, {
        message: "must contain at most 16 transfer headers",
      }),
    method: z.enum(["GET", "PUT"]),
    url: z.url().max(8_192).refine(isAllowedTransferUrl, {
      message: "must use HTTPS, or HTTP on an explicit loopback host",
    }),
  })
  .strict();

export const stashSummarySchema = z
  .object({
    committedAt: protocolV1DateTimeSchema,
    contentType: z.literal(stashContentType),
    sha256: sha256Schema,
    sizeBytes: z.number().int().positive().max(protocolV1MaxObjectBytes),
    stashId: stashIdSchema,
    storageVerification: z.enum([
      "presence-size-etag",
      "server-sha256",
    ]),
  })
  .strict();

export const syncPlanSchema = z
  .object({
    object: z
      .object({
        key: protocolV1ObjectKeySchema,
        sha256: sha256Schema,
        sizeBytes: z.number().int().positive().max(protocolV1MaxObjectBytes),
      })
      .strict(),
    receipt: z.string().min(1).max(4_096),
    state: z.enum([
      "already_committed",
      "object_present",
      "upload_required",
    ]),
    upload: transferGrantSchema.nullable(),
    version: z.literal("filecheap-sync/1"),
  })
  .strict()
  .superRefine((plan, context) => {
    if (plan.state === "upload_required" && plan.upload?.method !== "PUT") {
      context.addIssue({
        code: "custom",
        message: "upload_required plans must include a PUT grant",
        path: ["upload"],
      });
    }
    if (plan.state !== "upload_required" && plan.upload !== null) {
      context.addIssue({
        code: "custom",
        message: `${plan.state} plans must not include an upload grant`,
        path: ["upload"],
      });
    }
    if (!plan.object.key.endsWith(`/${plan.object.sha256}.fcheap`)) {
      context.addIssue({
        code: "custom",
        message: "object key must identify the declared SHA-256",
        path: ["object", "key"],
      });
    }
  });

export const commitPlanResponseSchema = z
  .object({
    requiresFullVerification: z.literal(true),
    stash: stashSummarySchema,
    version: z.literal("filecheap-sync/1"),
  })
  .strict();

export const downloadPlanSchema = z
  .object({
    expected: z
      .object({
        sha256: sha256Schema,
        sizeBytes: z.number().int().positive().max(protocolV1MaxObjectBytes),
      })
      .strict(),
    grant: transferGrantSchema.extend({ method: z.literal("GET") }).strict(),
    mustVerifySha256: z.literal(true),
    stashId: stashIdSchema,
    version: z.literal("filecheap-sync/1"),
  })
  .strict();

export const stashListSchema = z
  .object({
    stashes: z
      .array(stashSummarySchema)
      .max(protocolV1MaximumCatalogEntries),
    version: z.literal("filecheap-sync/1"),
  })
  .strict()
  .superRefine((catalog, context) => {
    const seen = new Set<string>();
    for (const [index, stash] of catalog.stashes.entries()) {
      if (seen.has(stash.stashId)) {
        context.addIssue({
          code: "custom",
          message: "catalog responses must not repeat a stash ID",
          path: ["stashes", index, "stashId"],
        });
      }
      seen.add(stash.stashId);
    }
  });

export type CreatePlanInput = z.infer<typeof createPlanSchema>;
export type CommitPlanInput = z.infer<typeof commitPlanSchema>;
export type CreateDownloadInput = z.infer<typeof createDownloadSchema>;
export type CommitPlanResponse = z.infer<typeof commitPlanResponseSchema>;
export type DownloadPlan = z.infer<typeof downloadPlanSchema>;
export type StashList = z.infer<typeof stashListSchema>;
export type StashSummary = z.infer<typeof stashSummarySchema>;
export type SyncPlan = z.infer<typeof syncPlanSchema>;

function isAllowedTransferUrl(value: string): boolean {
  const url = new URL(value);
  if (url.username || url.password || url.hash) return false;
  if (url.protocol === "https:") return true;
  if (url.protocol !== "http:") return false;
  return (
    url.hostname === "localhost" ||
    url.hostname === "127.0.0.1" ||
    url.hostname === "[::1]"
  );
}
