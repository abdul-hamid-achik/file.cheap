import { z } from "zod";

const ownerCheckEnvironmentSchema = z.object({
  FILECHEAP_OWNER_ACCOUNT_ID: z.string().regex(/^acc_[A-Za-z0-9_-]{8,64}$/u),
  FILECHEAP_OWNER_EMAIL: z.string().trim().email().max(320),
  MIGRATIONS_DATABASE_URL: z.string().trim().min(1).optional(),
  CONSOLE_OWNER_CHECK_DATABASE_URL: z.string().trim().min(1).optional(),
  VERCEL: z.string().optional(),
});

export class ConsoleOwnerCheckError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ConsoleOwnerCheckError";
  }
}

export type ConsoleOwnerCheckInput = Readonly<{
  ownerAccountId: string;
  ownerEmail: string;
  databaseUrl: string;
  databaseEnvironmentVariable:
    | "MIGRATIONS_DATABASE_URL"
    | "CONSOLE_OWNER_CHECK_DATABASE_URL";
}>;

export type ConsoleOwnerCandidate = Readonly<{
  id: unknown;
  email: unknown;
}>;

function assertPostgresUrl(
  value: string,
  variableName: ConsoleOwnerCheckInput["databaseEnvironmentVariable"],
): void {
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    throw new ConsoleOwnerCheckError(`${variableName} must be a valid Postgres URL.`);
  }

  if (parsed.protocol !== "postgres:" && parsed.protocol !== "postgresql:") {
    throw new ConsoleOwnerCheckError(`${variableName} must be a valid Postgres URL.`);
  }

  if (
    variableName === "MIGRATIONS_DATABASE_URL" &&
    (parsed.hostname.toLowerCase().includes("-pooler.") ||
      parsed.hostname.toLowerCase().endsWith("-pooler"))
  ) {
    throw new ConsoleOwnerCheckError(
      "MIGRATIONS_DATABASE_URL must use the direct, non-pooled database host.",
    );
  }
}

export function parseConsoleOwnerCheckInput(
  environment: Record<string, string | undefined>,
): ConsoleOwnerCheckInput {
  const result = ownerCheckEnvironmentSchema.safeParse(environment);
  if (!result.success) {
    throw new ConsoleOwnerCheckError("Console owner preflight configuration is invalid.");
  }

  if (result.data.VERCEL) {
    throw new ConsoleOwnerCheckError("Console owner preflight must run outside Vercel.");
  }

  const selected = result.data.MIGRATIONS_DATABASE_URL
    ? {
        databaseEnvironmentVariable: "MIGRATIONS_DATABASE_URL" as const,
        databaseUrl: result.data.MIGRATIONS_DATABASE_URL,
      }
    : result.data.CONSOLE_OWNER_CHECK_DATABASE_URL
      ? {
          databaseEnvironmentVariable: "CONSOLE_OWNER_CHECK_DATABASE_URL" as const,
          databaseUrl: result.data.CONSOLE_OWNER_CHECK_DATABASE_URL,
        }
      : undefined;

  if (!selected) {
    throw new ConsoleOwnerCheckError(
      "MIGRATIONS_DATABASE_URL or CONSOLE_OWNER_CHECK_DATABASE_URL is required.",
    );
  }

  assertPostgresUrl(selected.databaseUrl, selected.databaseEnvironmentVariable);

  return Object.freeze({
    ownerAccountId: result.data.FILECHEAP_OWNER_ACCOUNT_ID,
    ownerEmail: result.data.FILECHEAP_OWNER_EMAIL.toLowerCase(),
    ...selected,
  });
}

export function assertExactConsoleOwner(
  input: Pick<ConsoleOwnerCheckInput, "ownerAccountId" | "ownerEmail">,
  candidates: readonly ConsoleOwnerCandidate[],
): void {
  if (
    candidates.length !== 1 ||
    candidates[0]?.id !== input.ownerAccountId ||
    candidates[0]?.email !== input.ownerEmail
  ) {
    throw new ConsoleOwnerCheckError(
      "Console owner preflight failed: configuration does not match exactly one console_users row.",
    );
  }
}
