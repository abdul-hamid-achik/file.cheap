import { z } from "zod";

export const deviceAuthorizationInputSchema = z.object({
  clientName: z.string().trim().min(1).max(80),
  clientType: z.enum(["cli", "tv", "agent", "browser"]),
});

export const verificationEmailInputSchema = z.object({
  email: z.string().trim().email().max(320),
  userCode: z.string().trim().min(6).max(16),
});

export const authorizationDecisionInputSchema = z.object({
  decision: z.enum(["approve", "deny"]),
  email: z.string().trim().email().max(320),
  otp: z.string().regex(/^\d{6}$/u),
  userCode: z.string().trim().min(6).max(16),
});

export const deviceTokenInputSchema = z.object({
  deviceCode: z.string().min(32).max(256),
});

const refreshTokenSchema = z.string().regex(
  /^fcheap_refresh_[A-Za-z0-9_-]{43}$/u,
  "Invalid refresh token",
);

export const deviceRefreshInputSchema = z.object({
  nextRefreshToken: refreshTokenSchema,
  refreshToken: refreshTokenSchema,
  rotationId: z.string().regex(/^[A-Za-z0-9_-]{22,64}$/u),
}).refine(
  ({ nextRefreshToken, refreshToken }) => nextRefreshToken !== refreshToken,
  { message: "The replacement refresh token must be new", path: ["nextRefreshToken"] },
);

export const deviceFamilyIdSchema = z.string().uuid();

export const accessDeviceListQuerySchema = z.object({
  cursor: z.string().min(1).max(512).optional(),
  limit: z.coerce.number().int().min(1).max(50).default(20),
}).strict();

export const accessDeviceSchema = z.object({
  absoluteExpiresAt: z.string().datetime(),
  clientName: z.string().min(1).max(80),
  createdAt: z.string().datetime(),
  id: deviceFamilyIdSchema,
  idleExpiresAt: z.string().datetime(),
  lastRefreshedAt: z.string().datetime().nullable(),
  revokedAt: z.string().datetime().nullable(),
  status: z.enum(["active", "expired", "revoked"]),
}).strict();

export const accessDeviceListResponseSchema = z.object({
  devices: z.array(accessDeviceSchema).max(50),
  overview: z.object({
    active: z.number().int().nonnegative(),
    expiring: z.number().int().nonnegative(),
    inactive: z.number().int().nonnegative(),
    total: z.number().int().nonnegative(),
  }).strict(),
  pageInfo: z.object({
    endCursor: z.string().min(1).max(512).nullable(),
    hasNextPage: z.boolean(),
    limit: z.number().int().min(1).max(50),
  }).strict(),
  version: z.literal("filecheap-access/1"),
}).strict();

export const accessDeviceRevokeResponseSchema = z.object({
  id: deviceFamilyIdSchema,
  status: z.literal("revoked"),
}).strict();

export type DeviceAuthorizationInput = z.infer<
  typeof deviceAuthorizationInputSchema
>;
export type VerificationEmailInput = z.infer<
  typeof verificationEmailInputSchema
>;
export type AuthorizationDecisionInput = z.infer<
  typeof authorizationDecisionInputSchema
>;
export type DeviceRefreshInput = z.infer<typeof deviceRefreshInputSchema>;
export type AccessDevice = z.infer<typeof accessDeviceSchema>;
export type AccessDeviceListQuery = z.infer<
  typeof accessDeviceListQuerySchema
>;
export type AccessDeviceListResponse = z.infer<
  typeof accessDeviceListResponseSchema
>;
export type AccessDeviceOverview = AccessDeviceListResponse["overview"];

export type DeviceAuthorizationResponse = {
  deviceCode: string;
  expiresIn: number;
  interval: number;
  userCode: string;
  verificationUri: string;
};

export type DeviceTokenResponse = {
  accessToken: string;
  expiresIn: number;
  refreshExpiresIn: number;
  refreshToken: string;
  tokenType: "Bearer";
};
