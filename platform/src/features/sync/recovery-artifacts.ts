import { z } from "zod";

import { sha256Schema, stashIdSchema } from "@/features/sync/contracts";

const recoveryCardSchemaName = "filecheap.recovery-card.v1" as const;
const recoveryDrillReportSchemaName =
  "filecheap.recovery-drill-report.v1" as const;
const syncProtocolVersion = "filecheap-sync/1" as const;
const fallbackRecoveryFileName = "recovered.fcheap";
const maximumFileNameBytes = 255;

const safeRecoveryFileNameSchema = z
  .string()
  .min(1)
  .max(maximumFileNameBytes)
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

    const expectedDuration =
      Date.parse(report.completedAt) - Date.parse(report.startedAt);
    if (expectedDuration < 0) {
      context.addIssue({
        code: "custom",
        message: "completedAt must not be earlier than startedAt",
        path: ["completedAt"],
      });
    }
    if (report.durationMilliseconds !== expectedDuration) {
      context.addIssue({
        code: "custom",
        message: "durationMilliseconds must match the report timestamps",
        path: ["durationMilliseconds"],
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
  const normalizedName = replaceLoneSurrogates(name).normalize("NFC");
  const pathParts = normalizedName.replaceAll("\\", "/").split("/");
  let basename = "";
  for (let index = pathParts.length - 1; index >= 0; index -= 1) {
    if (pathParts[index]) {
      basename = pathParts[index];
      break;
    }
  }

  basename = basename
    .replace(
      /[\u0000-\u001f\u007f\u061c\u200e\u200f\u202a-\u202e\u2066-\u2069<>:"|?*]/g,
      "-",
    )
    .trim()
    .replace(/[. ]+$/g, "");
  basename = truncateUtf8(basename, maximumFileNameBytes).replace(/[. ]+$/g, "");

  if (
    !basename ||
    basename === "." ||
    basename === ".." ||
    isWindowsReservedName(basename)
  ) {
    return basename && isWindowsReservedName(basename)
      ? truncateUtf8(`_${basename}`, maximumFileNameBytes)
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

function replaceLoneSurrogates(value: string): string {
  let result = "";
  for (let index = 0; index < value.length; index += 1) {
    const codeUnit = value.charCodeAt(index);
    if (codeUnit >= 0xd800 && codeUnit <= 0xdbff) {
      const next = value.charCodeAt(index + 1);
      if (next >= 0xdc00 && next <= 0xdfff) {
        result += value[index] + value[index + 1];
        index += 1;
      } else {
        result += "-";
      }
    } else if (codeUnit >= 0xdc00 && codeUnit <= 0xdfff) {
      result += "-";
    } else {
      result += value[index];
    }
  }
  return result;
}

function truncateUtf8(value: string, maximumBytes: number): string {
  const encoder = new TextEncoder();
  let bytes = 0;
  let result = "";
  for (const character of value) {
    const characterBytes = encoder.encode(character).byteLength;
    if (bytes + characterBytes > maximumBytes) break;
    bytes += characterBytes;
    result += character;
  }
  return result;
}
