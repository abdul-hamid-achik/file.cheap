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
