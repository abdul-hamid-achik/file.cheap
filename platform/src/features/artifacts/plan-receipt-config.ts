import {
  PlanReceiptKeyring,
  type PlanReceiptKeyInput,
} from "@/features/artifacts/plan-receipts";

const activeKidVariable = "FILECHEAP_PLAN_RECEIPT_ACTIVE_KID";
const signingKeysVariable = "FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS";
const lookupKeysVariable = "FILECHEAP_PLAN_RECEIPT_LOOKUP_KEYS";
const maximumKeyringJsonBytes = 8_192;

/**
 * Parse the receipt keyring only at the private artifact-service boundary.
 * Public pages and health checks must never call this function.
 */
export function getPlanReceiptKeyring(
  env: Readonly<Record<string, string | undefined>> = process.env,
): PlanReceiptKeyring {
  const missing = [
    activeKidVariable,
    signingKeysVariable,
    lookupKeysVariable,
  ].filter((name) => !env[name]);
  if (missing.length > 0) {
    throw new Error(
      `Private artifact receipt keyring is not configured: missing ${missing.join(", ")}`,
    );
  }

  const signingKeys = parseKeyMap(
    signingKeysVariable,
    env[signingKeysVariable]!,
  );
  const lookupKeys = parseKeyMap(
    lookupKeysVariable,
    env[lookupKeysVariable]!,
  );
  const signingKids = Object.keys(signingKeys).sort();
  const lookupKids = Object.keys(lookupKeys).sort();
  if (
    signingKids.length !== lookupKids.length ||
    signingKids.some((kid, index) => kid !== lookupKids[index])
  ) {
    throw invalidKeyMap(
      "Signing and lookup key maps must contain exactly the same key IDs.",
    );
  }

  const keys: PlanReceiptKeyInput[] = signingKids.map((kid) => ({
    kid,
    lookupKey: lookupKeys[kid]!,
    signingKey: signingKeys[kid]!,
  }));
  return new PlanReceiptKeyring({
    activeKid: env[activeKidVariable]!,
    keys,
  });
}

function parseKeyMap(
  variableName: string,
  raw: string,
): Readonly<Record<string, Buffer>> {
  if (Buffer.byteLength(raw, "utf8") > maximumKeyringJsonBytes) {
    throw invalidKeyMap(`${variableName} exceeds its maximum encoded size.`);
  }

  let value: unknown;
  try {
    value = JSON.parse(raw);
  } catch {
    throw invalidKeyMap(`${variableName} must be a JSON object.`);
  }
  if (
    typeof value !== "object" ||
    value === null ||
    Array.isArray(value)
  ) {
    throw invalidKeyMap(`${variableName} must be a JSON object.`);
  }

  const entries = Object.entries(value);
  if (entries.length < 1 || entries.length > 16) {
    throw invalidKeyMap(`${variableName} must contain between 1 and 16 keys.`);
  }

  const result: Record<string, Buffer> = {};
  for (const [kid, encoded] of entries) {
    if (
      !/^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$/u.test(kid) ||
      typeof encoded !== "string" ||
      !/^[A-Za-z0-9_-]{43,86}$/u.test(encoded)
    ) {
      throw invalidKeyMap(
        `${variableName} must map safe key IDs to canonical base64url key material.`,
      );
    }
    const decoded = Buffer.from(encoded, "base64url");
    if (
      decoded.byteLength < 32 ||
      decoded.byteLength > 64 ||
      decoded.toString("base64url") !== encoded
    ) {
      throw invalidKeyMap(
        `${variableName} keys must decode to between 32 and 64 bytes.`,
      );
    }
    result[kid] = decoded;
  }
  return Object.freeze(result);
}

function invalidKeyMap(detail: string): Error {
  return new Error(`Invalid private artifact receipt keyring. ${detail}`);
}
