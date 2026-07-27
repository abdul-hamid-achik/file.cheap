import { Resend } from "resend";

import type { AuthMailer } from "@/features/auth/service";

export class ResendAuthMailer implements AuthMailer {
  private readonly resend: Resend;

  constructor(apiKey: string, private readonly from: string) {
    this.resend = new Resend(apiKey);
  }

  async sendVerification(input: Parameters<AuthMailer["sendVerification"]>[0]): Promise<void> {
    const result = await this.resend.emails.send({
      from: this.from,
      headers: { "Referrer-Policy": "no-referrer" },
      html: verificationHtml(input),
      subject: `${input.otp} is your file.cheap verification code`,
      text: [
        `Your file.cheap verification code is ${input.otp}.`,
        `Device: ${input.clientName}`,
        `Pairing code: ${input.userCode}`,
        `Review and approve at ${input.verificationUri}`,
        "This request expires in 10 minutes. Opening this email does not approve the device.",
      ].join("\n\n"),
      to: input.email,
    }, { idempotencyKey: input.idempotencyKey });
    if (result.error || !result.data?.id) {
      // A resolved SDK call without a provider message id is not sufficient
      // evidence of acceptance and must not activate the OTP.
      throw new Error("Resend did not accept the verification email");
    }
  }
}

function verificationHtml(input: Parameters<AuthMailer["sendVerification"]>[0]): string {
  const clientName = escapeHtml(input.clientName);
  const otp = escapeHtml(input.otp);
  const userCode = escapeHtml(input.userCode);
  const uri = escapeHtml(input.verificationUri);
  return `<!doctype html><html><body style="background:#15140f;color:#f4efe4;font-family:ui-sans-serif,system-ui;padding:32px"><main style="max-width:520px;margin:auto;background:#1d1c17;border:1px solid #38342c;border-radius:16px;padding:28px"><p style="color:#f1774f;font-family:ui-monospace,monospace">file.cheap / device verification</p><h1 style="font-size:24px">Verify ${clientName}</h1><p>Enter this one-time code on the approval page:</p><p style="font:700 34px ui-monospace,monospace;letter-spacing:.16em">${otp}</p><p>Pairing code: <strong>${userCode}</strong></p><p><a style="color:#f1774f" href="${uri}" rel="noreferrer">Review the request</a></p><p style="color:#aaa397">Opening this email does not approve the device. The request expires in 10 minutes.</p></main></body></html>`;
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
