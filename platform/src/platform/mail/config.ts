import { z } from "zod";

import {
  FILECHEAP_FORWARDING_EMAIL,
  FILECHEAP_INBOUND_EMAIL,
} from "@/features/mail/inbound";
import { PlatformError } from "@/shared/errors/platform-error";

type EmailEnvironment = Readonly<Record<string, string | undefined>>;

export interface EmailRuntimeConfig {
  receiveApiKey: string;
  webhookSecret: string;
  forwardTo: string;
}

const emailConfigSchema = z.object({
  RESEND_RECEIVE_API_KEY: z
    .string()
    .min(20)
    .max(512)
    .regex(/^re_[A-Za-z0-9_-]+$/u),
  RESEND_WEBHOOK_SECRET: z
    .string()
    .min(20)
    .max(512)
    .regex(/^whsec_[A-Za-z0-9+/=_-]+$/u),
  RESEND_FORWARD_TO: z.string().email().max(320),
});

function configurationError(): PlatformError {
  return new PlatformError({
    code: "email_not_configured",
    detail: "Inbound email delivery is not configured.",
    status: 503,
    title: "Email unavailable",
  });
}

export function getEmailRuntimeConfig(
  env: EmailEnvironment = process.env,
): EmailRuntimeConfig {
  const parsed = emailConfigSchema.safeParse(env);
  if (!parsed.success) throw configurationError();

  const forwardTo = parsed.data.RESEND_FORWARD_TO.toLowerCase();
  if (
    forwardTo === FILECHEAP_INBOUND_EMAIL ||
    forwardTo === FILECHEAP_FORWARDING_EMAIL
  ) {
    throw configurationError();
  }

  return Object.freeze({
    receiveApiKey: parsed.data.RESEND_RECEIVE_API_KEY,
    webhookSecret: parsed.data.RESEND_WEBHOOK_SECRET,
    forwardTo,
  });
}
