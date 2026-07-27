import {
  createHmac,
  randomBytes,
  timingSafeEqual,
} from "node:crypto";

const artifactIdPattern = /^art_[A-Za-z0-9_-]{16,96}$/;
const keyIdPattern = /^[A-Za-z0-9][A-Za-z0-9._-]{0,31}$/;
const receiptPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const base64UrlPattern = /^[A-Za-z0-9_-]+$/;

const receiptDomain = Buffer.from("file.cheap/plan-receipt/v1\0", "utf8");
const lookupDomain = Buffer.from("file.cheap/plan-receipt-lookup/v1\0", "utf8");
const nonceBytes = 32;
const lookupBytes = 32;
const minimumKeyBytes = 32;
const maximumKeyBytes = 64;
const maximumKeys = 16;

export const planReceiptSchemeV1 = "hmac-sha256-v1" as const;
export const legacyPlanReceiptLookupSchemeV1 =
  "legacy-random-hmac-sha256-v1" as const;

declare const planReceiptNonceBrand: unique symbol;
declare const planReceiptLookupBrand: unique symbol;

export type PlanReceiptNonce = string & {
  readonly [planReceiptNonceBrand]: true;
};

export type PlanReceiptLookup = string & {
  readonly [planReceiptLookupBrand]: true;
};

export type PlanReceiptKeyInput = Readonly<{
  kid: string;
  lookupKey: Uint8Array;
  signingKey: Uint8Array;
}>;

export type PlanReceiptKeyringInput = Readonly<{
  activeKid: string;
  keys: readonly PlanReceiptKeyInput[];
}>;

export type IssuedPlanReceipt = Readonly<{
  receipt: string;
  receiptKid: string;
  receiptLookup: PlanReceiptLookup;
  receiptNonce: PlanReceiptNonce;
  receiptScheme: typeof planReceiptSchemeV1;
}>;

export type PlanReceiptLookupCandidate = Readonly<{
  receiptKid: string;
  receiptLookup: PlanReceiptLookup;
}>;

export type StoredPlanReceiptMaterial = Readonly<{
  artifactId: string;
  receiptKid: string;
  receiptLookup: string;
  receiptNonce: string;
}>;

export type StoredPlanReceiptLookup = Readonly<{
  receiptKid: string;
  receiptLookup: string;
}>;

export type PlanReceiptConfigurationErrorCode =
  | "active_key_missing"
  | "duplicate_key_id"
  | "invalid_active_key_id"
  | "invalid_key_id"
  | "invalid_key_material"
  | "key_material_reused"
  | "key_missing"
  | "keyring_size";

/** A fail-fast configuration error that never includes secret key material. */
export class PlanReceiptConfigurationError extends Error {
  constructor(
    public readonly code: PlanReceiptConfigurationErrorCode,
    message: string,
  ) {
    super(message);
    this.name = "PlanReceiptConfigurationError";
  }
}

type KeyMaterial = Readonly<{
  lookupKey: Buffer;
  signingKey: Buffer;
}>;

type KeyringState = Readonly<{
  keys: ReadonlyMap<string, KeyMaterial>;
}>;

const keyringStates = new WeakMap<PlanReceiptKeyring, KeyringState>();

/**
 * An opaque, validated keyring. Callers can inspect key IDs for rotation and
 * lookup, but key bytes cannot be read back out of this object.
 */
export class PlanReceiptKeyring {
  readonly activeKid: string;
  readonly kids: readonly string[];

  constructor(input: PlanReceiptKeyringInput) {
    assertKeyId(input.activeKid, "invalid_active_key_id");
    if (input.keys.length < 1 || input.keys.length > maximumKeys) {
      throw new PlanReceiptConfigurationError(
        "keyring_size",
        `Plan receipt keyrings must contain between 1 and ${maximumKeys} keys.`,
      );
    }

    const keys = new Map<string, KeyMaterial>();
    const allKeyMaterial: Buffer[] = [];
    for (const entry of input.keys) {
      assertKeyId(entry.kid, "invalid_key_id");
      if (keys.has(entry.kid)) {
        throw new PlanReceiptConfigurationError(
          "duplicate_key_id",
          `Plan receipt key ID '${entry.kid}' is duplicated.`,
        );
      }

      const signingKey = copyAndValidateKey(entry.signingKey);
      const lookupKey = copyAndValidateKey(entry.lookupKey);
      if (
        allKeyMaterial.some((existing) => equalKeyMaterial(existing, signingKey)) ||
        allKeyMaterial.some((existing) => equalKeyMaterial(existing, lookupKey)) ||
        equalKeyMaterial(signingKey, lookupKey)
      ) {
        throw new PlanReceiptConfigurationError(
          "key_material_reused",
          "Plan receipt signing and lookup keys must be unique across the keyring.",
        );
      }

      allKeyMaterial.push(signingKey, lookupKey);
      keys.set(entry.kid, Object.freeze({ lookupKey, signingKey }));
    }

    if (!keys.has(input.activeKid)) {
      throw new PlanReceiptConfigurationError(
        "active_key_missing",
        `The active plan receipt key ID '${input.activeKid}' is not loaded.`,
      );
    }

    this.activeKid = input.activeKid;
    this.kids = Object.freeze([
      input.activeKid,
      ...[...keys.keys()]
        .filter((kid) => kid !== input.activeKid)
        .sort((left, right) => left.localeCompare(right)),
    ]);
    keyringStates.set(this, Object.freeze({ keys }));
    Object.freeze(this);
  }
}

/** Generate the only random value that must be persisted for reconstruction. */
export function generatePlanReceiptNonce(): PlanReceiptNonce {
  return encodeNonce(randomBytes(nonceBytes));
}

/** Validate and brand a persisted, canonical 32-byte base64url nonce. */
export function parsePlanReceiptNonce(value: string): PlanReceiptNonce {
  const decoded = decodeCanonicalBase64Url(value, nonceBytes);
  return encodeNonce(decoded);
}

/** Validate and brand a persisted, canonical SHA-256 lookup digest. */
export function parsePlanReceiptLookup(value: string): PlanReceiptLookup {
  const decoded = decodeCanonicalBase64Url(value, lookupBytes);
  return encodeLookup(decoded);
}

/**
 * Issue a contract-compatible UUID using the active key. Supplying the same
 * artifact ID and nonce to the same key produces exactly the same receipt.
 */
export function issuePlanReceipt(
  keyring: PlanReceiptKeyring,
  artifactId: string,
  nonce: PlanReceiptNonce = generatePlanReceiptNonce(),
): IssuedPlanReceipt {
  const receiptKid = keyring.activeKid;
  const receipt = reconstructPlanReceipt(keyring, {
    artifactId,
    receiptKid,
    receiptNonce: nonce,
  });
  return Object.freeze({
    receipt,
    receiptKid,
    receiptLookup: derivePlanReceiptLookup(keyring, receiptKid, receipt),
    receiptNonce: nonce,
    receiptScheme: planReceiptSchemeV1,
  });
}

/** Reconstruct a receipt for an idempotent replay without storing the bearer. */
export function reconstructPlanReceipt(
  keyring: PlanReceiptKeyring,
  material: Readonly<{
    artifactId: string;
    receiptKid: string;
    receiptNonce: string;
  }>,
): string {
  assertArtifactId(material.artifactId);
  const nonce = decodeCanonicalBase64Url(material.receiptNonce, nonceBytes);
  const key = requireKey(keyring, material.receiptKid);
  const digest = receiptDigest(key.signingKey, material.artifactId, nonce);
  return formatReceiptUuid(digest.subarray(0, 16));
}

/** Derive the non-bearer database lookup value using a separate HMAC key. */
export function derivePlanReceiptLookup(
  keyring: PlanReceiptKeyring,
  receiptKid: string,
  receipt: string,
): PlanReceiptLookup {
  const key = requireKey(keyring, receiptKid);
  return encodeLookup(lookupDigest(key.lookupKey, parseReceiptUuid(receipt)));
}

/**
 * Build bounded lookup candidates for all retained rotation keys. The active
 * key is first; callers query by the exact (kid, digest) pair.
 */
export function derivePlanReceiptLookupCandidates(
  keyring: PlanReceiptKeyring,
  receipt: string,
): readonly PlanReceiptLookupCandidate[] {
  const receiptBytes = parseReceiptUuid(receipt);
  return Object.freeze(
    keyring.kids.map((receiptKid) => {
      const key = requireKey(keyring, receiptKid);
      return Object.freeze({
        receiptKid,
        receiptLookup: encodeLookup(lookupDigest(key.lookupKey, receiptBytes)),
      });
    }),
  );
}

/** Verify a lookup-only legacy receipt after an online keyed backfill. */
export function verifyPlanReceiptLookup(
  keyring: PlanReceiptKeyring,
  receipt: string,
  stored: StoredPlanReceiptLookup,
): boolean {
  const key = getKey(keyring, stored.receiptKid);
  if (!key) return false;

  try {
    const receiptBytes = parseReceiptUuid(receipt);
    const storedLookup = decodeCanonicalBase64Url(
      stored.receiptLookup,
      lookupBytes,
    );
    return timingSafeEqual(storedLookup, lookupDigest(key.lookupKey, receiptBytes));
  } catch {
    return false;
  }
}

/**
 * Verify both independent HMAC bindings in constant time after a repository
 * candidate match. Unknown/retired keys and malformed stored values fail
 * closed without exposing which check failed.
 */
export function verifyPlanReceipt(
  keyring: PlanReceiptKeyring,
  receipt: string,
  stored: StoredPlanReceiptMaterial,
): boolean {
  const key = getKey(keyring, stored.receiptKid);
  if (!key) return false;

  try {
    assertArtifactId(stored.artifactId);
    const receiptBytes = parseReceiptUuid(receipt);
    const nonce = decodeCanonicalBase64Url(stored.receiptNonce, nonceBytes);
    const storedLookup = decodeCanonicalBase64Url(
      stored.receiptLookup,
      lookupBytes,
    );
    const expectedReceipt = receiptDigest(
      key.signingKey,
      stored.artifactId,
      nonce,
    ).subarray(0, 16);
    setUuidVersionAndVariant(expectedReceipt);
    const expectedLookup = lookupDigest(key.lookupKey, receiptBytes);

    // Do not short-circuit: both fixed-size comparisons run for a loaded key.
    const receiptMatches = timingSafeEqual(receiptBytes, expectedReceipt);
    const lookupMatches = timingSafeEqual(storedLookup, expectedLookup);
    return Boolean(Number(receiptMatches) & Number(lookupMatches));
  } catch {
    return false;
  }
}

function getKey(
  keyring: PlanReceiptKeyring,
  receiptKid: string,
): KeyMaterial | undefined {
  return keyringState(keyring).keys.get(receiptKid);
}

function requireKey(
  keyring: PlanReceiptKeyring,
  receiptKid: string,
): KeyMaterial {
  const key = getKey(keyring, receiptKid);
  if (!key) {
    throw new PlanReceiptConfigurationError(
      "key_missing",
      "The requested plan receipt key is not loaded.",
    );
  }
  return key;
}

function keyringState(keyring: PlanReceiptKeyring): KeyringState {
  const state = keyringStates.get(keyring);
  if (!state) {
    throw new PlanReceiptConfigurationError(
      "key_missing",
      "The plan receipt keyring was not initialized by this module.",
    );
  }
  return state;
}

function receiptDigest(
  signingKey: Buffer,
  artifactId: string,
  nonce: Buffer,
): Buffer {
  const artifactBytes = Buffer.from(artifactId, "utf8");
  const artifactLength = Buffer.allocUnsafe(4);
  artifactLength.writeUInt32BE(artifactBytes.byteLength);
  return createHmac("sha256", signingKey)
    .update(receiptDomain)
    .update(artifactLength)
    .update(artifactBytes)
    .update(nonce)
    .digest();
}

function lookupDigest(lookupKey: Buffer, receiptBytes: Buffer): Buffer {
  return createHmac("sha256", lookupKey)
    .update(lookupDomain)
    .update(receiptBytes)
    .digest();
}

function formatReceiptUuid(bytes: Uint8Array): string {
  const value = Buffer.from(bytes);
  setUuidVersionAndVariant(value);
  const hex = value.toString("hex");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

function parseReceiptUuid(value: string): Buffer {
  if (!receiptPattern.test(value)) {
    throw new TypeError("Plan receipt must be a canonical UUID.");
  }
  return Buffer.from(value.replaceAll("-", ""), "hex");
}

function setUuidVersionAndVariant(value: Buffer): void {
  const versionByte = value[6];
  const variantByte = value[8];
  if (versionByte === undefined || variantByte === undefined) {
    throw new TypeError("Plan receipt digest is too short.");
  }
  value[6] = (versionByte & 0x0f) | 0x40;
  value[8] = (variantByte & 0x3f) | 0x80;
}

function copyAndValidateKey(value: Uint8Array): Buffer {
  if (
    !(value instanceof Uint8Array) ||
    value.byteLength < minimumKeyBytes ||
    value.byteLength > maximumKeyBytes
  ) {
    throw new PlanReceiptConfigurationError(
      "invalid_key_material",
      `Plan receipt keys must contain between ${minimumKeyBytes} and ${maximumKeyBytes} bytes.`,
    );
  }
  return Buffer.from(value);
}

function equalKeyMaterial(left: Buffer, right: Buffer): boolean {
  return left.byteLength === right.byteLength && timingSafeEqual(left, right);
}

function assertKeyId(
  value: string,
  code: Extract<
    PlanReceiptConfigurationErrorCode,
    "invalid_active_key_id" | "invalid_key_id"
  >,
): void {
  if (!keyIdPattern.test(value)) {
    throw new PlanReceiptConfigurationError(
      code,
      "Plan receipt key IDs must be 1-32 safe ASCII characters.",
    );
  }
}

function assertArtifactId(value: string): void {
  if (!artifactIdPattern.test(value)) {
    throw new TypeError("Plan receipt artifact ID is invalid.");
  }
}

function decodeCanonicalBase64Url(value: string, size: number): Buffer {
  if (!base64UrlPattern.test(value)) {
    throw new TypeError("Plan receipt material must be canonical base64url.");
  }
  const decoded = Buffer.from(value, "base64url");
  if (decoded.byteLength !== size || decoded.toString("base64url") !== value) {
    throw new TypeError("Plan receipt material has an invalid length or encoding.");
  }
  return decoded;
}

function encodeNonce(value: Uint8Array): PlanReceiptNonce {
  return Buffer.from(value).toString("base64url") as PlanReceiptNonce;
}

function encodeLookup(value: Uint8Array): PlanReceiptLookup {
  return Buffer.from(value).toString("base64url") as PlanReceiptLookup;
}
