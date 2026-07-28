import { describe, expect, test } from "bun:test";

import { buildVerificationEmail } from "@/platform/mail/resend-auth-mailer";

describe("console verification email", () => {
  test("keeps the request recognizable without exposing the local device name", () => {
    const message = buildVerificationEmail({
      clientName: "Personal-MacBook.local",
      email: "owner@example.test",
      idempotencyKey: "verification-fixture",
      otp: "322362",
      userCode: "K7XM-VPBV",
      verificationUri: "https://file.cheap/console/activate",
    });

    expect(message.subject).toBe("Your file.cheap sign-in code");
    expect(message.html).toContain("322362");
    expect(message.html).toContain("K7XM-VPBV");
    expect(message.html).toContain('href="https://file.cheap/console/activate"');
    expect(message.text).toContain("https://file.cheap/console/activate");
    expect(`${message.subject}\n${message.html}\n${message.text}`).not.toContain(
      "Personal-MacBook.local",
    );
  });

  test("escapes dynamic HTML while preserving the plain-text alternative", () => {
    const message = buildVerificationEmail({
      clientName: "CLI",
      email: "owner@example.test",
      idempotencyKey: "verification-escaping-fixture",
      otp: "123456",
      userCode: "ABCD-1234",
      verificationUri: 'https://file.cheap/console/activate?next="owner"&mode=login',
    });

    expect(message.html).toContain("?next=&quot;owner&quot;&amp;mode=login");
    expect(message.html).not.toContain('<script');
    expect(message.text).toContain('?next="owner"&mode=login');
  });
});
