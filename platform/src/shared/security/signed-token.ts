import { createHmac, timingSafeEqual } from "node:crypto";

import { z } from "zod";

import { PlatformError } from "@/shared/errors/platform-error";

export const signedPayloadSchema = z.discriminatedUnion("kind", [
  z.object({
    contentType: z.string().min(1),
    exp: z.number().int().positive(),
    key: z.string().min(1),
    kind: z.literal("upload"),
    sha256: z.string().regex(/^[a-f0-9]{64}$/),
    sizeBytes: z.number().int().nonnegative(),
  }),
  z.object({
    exp: z.number().int().positive(),
    key: z.string().min(1),
    kind: z.literal("download"),
  }),
  z.object({
    contentType: z.string().min(1),
    exp: z.number().int().positive(),
    key: z.string().min(1),
    kind: z.literal("commit"),
    sha256: z.string().regex(/^[a-f0-9]{64}$/),
    sizeBytes: z.number().int().nonnegative(),
    stashId: z.string().min(1),
  }),
]);

export type SignedPayload = z.infer<typeof signedPayloadSchema>;

export function signPayload(payload: SignedPayload, secret: string): string {
  const encodedPayload = Buffer.from(JSON.stringify(payload)).toString("base64url");
  const signature = signatureFor(encodedPayload, secret);
  return `${encodedPayload}.${signature}`;
}

export function verifyPayload(token: string, secret: string): SignedPayload {
  const [encodedPayload, receivedSignature, extra] = token.split(".");
  if (!encodedPayload || !receivedSignature || extra) {
    throw invalidToken();
  }

  const expectedSignature = signatureFor(encodedPayload, secret);
  const receivedBytes = Buffer.from(receivedSignature);
  const expectedBytes = Buffer.from(expectedSignature);
  if (
    receivedBytes.length !== expectedBytes.length ||
    !timingSafeEqual(receivedBytes, expectedBytes)
  ) {
    throw invalidToken();
  }

  try {
    const decoded = JSON.parse(Buffer.from(encodedPayload, "base64url").toString("utf8"));
    const payload = signedPayloadSchema.parse(decoded);
    if (payload.exp <= Date.now()) {
      throw new PlatformError({
        code: "expired_grant",
        detail: "The transfer grant has expired. Request a new plan.",
        status: 410,
        title: "Expired grant",
      });
    }
    return payload;
  } catch (error) {
    if (error instanceof PlatformError) {
      throw error;
    }
    throw invalidToken();
  }
}

function signatureFor(encodedPayload: string, secret: string): string {
  return createHmac("sha256", secret).update(encodedPayload).digest("base64url");
}

function invalidToken(): PlatformError {
  return new PlatformError({
    code: "invalid_grant",
    detail: "The transfer grant is invalid.",
    status: 403,
    title: "Invalid grant",
  });
}
