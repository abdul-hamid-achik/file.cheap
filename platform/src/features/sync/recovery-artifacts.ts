import { z } from "zod";

import { sha256Schema, stashIdSchema } from "@/features/sync/contracts";

const recoveryCardSchemaName = "filecheap.recovery-card.v1" as const;
const recoveryDrillReportSchemaName =
  "filecheap.recovery-drill-report.v1" as const;
const syncProtocolVersion = "filecheap-sync/1" as const;
const fallbackRecoveryFileName = "recovered.fcheap";
const maximumFileNameLength = 255;

const safeRecoveryFileNameSchema = z
  .string()
  .min(1)
  .max(maximumFileNameLength)
  .refine((name) => sanitizeRecoveryFileName(name) === name, {
    message: "must be a safe basename",
  });

export const recoveryCardSchema = z
  .object({
    schema: z.literal(recoveryCardSchemaName),
    protocolVersion: z.literal(syncProtocolVersion),
    stashId: stashIdSchema,
    originalFileName: safeRecoveryFileNameSchema,
    sha256: sha256Schema,
    sizeBytes: z.number().int().positive(),
    committedAt: z.iso.datetime(),
  })
  .strict();

export type RecoveryCard = z.infer<typeof recoveryCardSchema>;

export type CreateRecoveryCardInput = Omit<
  RecoveryCard,
  "schema" | "protocolVersion" | "originalFileName"
> & {
  originalFileName: string;
};

export const recoveryDrillReportSchema = z
  .object({
    attemptId: z.uuid(),
    evidenceType: z.literal("local-client-observation"),
    schema: z.literal(recoveryDrillReportSchemaName),
    stashId: stashIdSchema,
    sha256: sha256Schema,
    sizeBytes: z.number().int().positive(),
    startedAt: z.iso.datetime(),
    completedAt: z.iso.datetime(),
    durationMilliseconds: z.number().int().nonnegative(),
    checks: z
      .object({
        download: z.literal("passed"),
        selectedFileByteEquivalent: z.literal("passed"),
      })
      .strict(),
    recoveryCard: recoveryCardSchema,
    result: z.literal("verified"),
    tamperEvident: z.literal(false),
  })
  .strict()
  .superRefine((report, context) => {
    if (
      report.stashId !== report.recoveryCard.stashId ||
      report.sha256 !== report.recoveryCard.sha256 ||
      report.sizeBytes !== report.recoveryCard.sizeBytes
    ) {
      context.addIssue({
        code: "custom",
        message: "report identity must match its recovery card",
      });
    }
  });

export type RecoveryDrillReport = z.infer<
  typeof recoveryDrillReportSchema
>;

export type CreateRecoveryDrillReportInput = Omit<
  RecoveryDrillReport,
  | "schema"
  | "durationMilliseconds"
  | "checks"
  | "evidenceType"
  | "result"
  | "tamperEvident"
>;

export function createRecoveryCard(
  input: CreateRecoveryCardInput,
): RecoveryCard {
  return recoveryCardSchema.parse({
    ...input,
    originalFileName: sanitizeRecoveryFileName(input.originalFileName),
    protocolVersion: syncProtocolVersion,
    schema: recoveryCardSchemaName,
  });
}

export function parseRecoveryCard(input: unknown | string): RecoveryCard {
  return recoveryCardSchema.parse(parseArtifactInput(input));
}

export function serializeRecoveryCard(card: RecoveryCard): string {
  return serializeArtifact(recoveryCardSchema.parse(card));
}

export function createRecoveryDrillReport(
  input: CreateRecoveryDrillReportInput,
): RecoveryDrillReport {
  return recoveryDrillReportSchema.parse({
    ...input,
    checks: {
      download: "passed",
      selectedFileByteEquivalent: "passed",
    },
    durationMilliseconds:
      Date.parse(input.completedAt) - Date.parse(input.startedAt),
    evidenceType: "local-client-observation",
    result: "verified",
    schema: recoveryDrillReportSchemaName,
    tamperEvident: false,
  });
}

export function recoveryCardIdentity(card: RecoveryCard): string {
  return serializeRecoveryCard(recoveryCardSchema.parse(card));
}

export function parseRecoveryDrillReport(
  input: unknown | string,
): RecoveryDrillReport {
  return recoveryDrillReportSchema.parse(parseArtifactInput(input));
}

export function serializeRecoveryDrillReport(
  report: RecoveryDrillReport,
): string {
  return serializeArtifact(recoveryDrillReportSchema.parse(report));
}

export function sanitizeRecoveryFileName(name: string): string {
  const pathParts = name.replaceAll("\\", "/").split("/");
  let basename = "";
  for (let index = pathParts.length - 1; index >= 0; index -= 1) {
    if (pathParts[index]) {
      basename = pathParts[index];
      break;
    }
  }

  basename = basename
    .replace(/[\u0000-\u001f\u007f<>:"|?*]/g, "-")
    .trim()
    .replace(/[. ]+$/g, "")
    .slice(0, maximumFileNameLength);

  if (
    !basename ||
    basename === "." ||
    basename === ".." ||
    isWindowsReservedName(basename)
  ) {
    return basename && isWindowsReservedName(basename)
      ? `_${basename}`.slice(0, maximumFileNameLength)
      : fallbackRecoveryFileName;
  }

  return basename;
}

function parseArtifactInput(input: unknown | string): unknown {
  return typeof input === "string" ? JSON.parse(input) : input;
}

function serializeArtifact(value: unknown): string {
  return `${JSON.stringify(value, null, 2)}\n`;
}

function isWindowsReservedName(name: string): boolean {
  return /^(con|prn|aux|nul|com[1-9]|lpt[1-9])(?:\..*)?$/i.test(name);
}
