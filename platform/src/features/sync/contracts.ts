import { z } from "zod";

export const stashContentType = "application/vnd.filecheap.stash" as const;
export const protocolV1MaxObjectBytes = 64 * 1024 * 1024;

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

export type CreatePlanInput = z.infer<typeof createPlanSchema>;
export type CommitPlanInput = z.infer<typeof commitPlanSchema>;
export type CreateDownloadInput = z.infer<typeof createDownloadSchema>;
