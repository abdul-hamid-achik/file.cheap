import { Resend } from "resend";

import type { AuthMailer } from "@/features/auth/service";

export class ResendAuthMailer implements AuthMailer {
  private readonly resend: Resend;

  constructor(apiKey: string, private readonly from: string) {
    this.resend = new Resend(apiKey);
  }

  async sendVerification(input: Parameters<AuthMailer["sendVerification"]>[0]): Promise<void> {
    const message = buildVerificationEmail(input);
    const result = await this.resend.emails.send({
      from: this.from,
      ...message,
      to: input.email,
    }, { idempotencyKey: input.idempotencyKey });
    if (result.error || !result.data?.id) {
      // A resolved SDK call without a provider message id is not sufficient
      // evidence of acceptance and must not activate the OTP.
      throw new Error("Resend did not accept the verification email");
    }
  }
}

export function buildVerificationEmail(
  input: Parameters<AuthMailer["sendVerification"]>[0],
): Readonly<{ html: string; subject: string; text: string }> {
  const otp = escapeHtml(input.otp);
  const userCode = escapeHtml(input.userCode);
  const uri = escapeHtml(input.verificationUri);
  return Object.freeze({
    html: `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Your file.cheap sign-in code</title></head><body style="margin:0;background:#f6f4ee;color:#24231f;font-family:Arial,sans-serif"><main style="max-width:520px;margin:0 auto;padding:40px 24px"><p style="margin:0 0 24px;color:#6d675d;font-size:14px">file.cheap</p><h1 style="margin:0 0 16px;font-size:24px;line-height:1.25">Your sign-in code</h1><p style="margin:0 0 20px;line-height:1.6">Use this one-time code to continue with file.cheap:</p><p style="margin:0 0 24px;font-family:ui-monospace,monospace;font-size:32px;font-weight:700;letter-spacing:.14em">${otp}</p><p style="margin:0 0 24px;line-height:1.6">Request code: <strong>${userCode}</strong></p><p style="margin:0 0 28px"><a style="color:#a43f24" href="${uri}">Continue to file.cheap</a></p><p style="margin:0;color:#6d675d;font-size:14px;line-height:1.6">This code expires in 10 minutes. If you did not request it, you can ignore this email.</p></main></body></html>`,
    subject: "Your file.cheap sign-in code",
    text: [
      "Use this one-time code to continue with file.cheap.",
      `One-time code: ${input.otp}`,
      `Request code: ${input.userCode}`,
      `Open file.cheap: ${input.verificationUri}`,
      "This code expires in 10 minutes. If you did not request it, you can ignore this email.",
    ].join("\n\n"),
  });
}

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/gu, (character) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;",
  })[character]!);
}
