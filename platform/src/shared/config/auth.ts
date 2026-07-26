import { z } from "zod";

const authEnvironmentSchema = z.object({
  FILECHEAP_AUTH_SECRET: z.string().min(32).max(256),
  FILECHEAP_OWNER_EMAIL: z.string().trim().email().max(320),
  FILECHEAP_OWNER_ACCOUNT_ID: z.string().regex(/^acc_[A-Za-z0-9_-]{8,64}$/u),
  PLATFORM_PUBLIC_URL: z.url().default("http://127.0.0.1:3100"),
  RESEND_AUTH_FROM: z.string().trim().min(3).max(320),
  RESEND_AUTH_SEND_API_KEY: z.string().min(1),
});

export type AuthConfig = {
  allowedEmails: readonly string[];
  from: string;
  ownerAccountId: string;
  publicUrl: string;
  resendApiKey: string;
  secret: string;
};

let cached: AuthConfig | undefined;

export function getAuthConfig(): AuthConfig {
  if (cached) return cached;
  const parsed = authEnvironmentSchema.parse(process.env);
  const publicUrl = new URL(parsed.PLATFORM_PUBLIC_URL);
  if (publicUrl.username || publicUrl.password || publicUrl.pathname !== "/" || publicUrl.search || publicUrl.hash) {
    throw new Error("PLATFORM_PUBLIC_URL must be a bare origin for console authentication");
  }
  if ((process.env.NODE_ENV === "production" || process.env.VERCEL) && publicUrl.protocol !== "https:") {
    throw new Error("Console authentication requires an HTTPS PLATFORM_PUBLIC_URL in production");
  }
  cached = {
    allowedEmails: Object.freeze([parsed.FILECHEAP_OWNER_EMAIL.toLowerCase()]),
    from: parsed.RESEND_AUTH_FROM,
    ownerAccountId: parsed.FILECHEAP_OWNER_ACCOUNT_ID,
    publicUrl: publicUrl.origin,
    resendApiKey: parsed.RESEND_AUTH_SEND_API_KEY,
    secret: parsed.FILECHEAP_AUTH_SECRET,
  };
  return cached;
}

export function resetAuthConfigForTests(): void {
  cached = undefined;
}
