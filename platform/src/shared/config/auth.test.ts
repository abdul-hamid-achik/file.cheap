import { afterEach, expect, test } from "bun:test";

import { getAuthConfig, resetAuthConfigForTests } from "@/shared/config/auth";

const original = { ...process.env };

afterEach(() => {
  for (const key of Object.keys(process.env)) if (!(key in original)) delete process.env[key];
  for (const [key, value] of Object.entries(original)) process.env[key] = value;
  resetAuthConfigForTests();
});

test("loads the single-owner console boundary independently", () => {
  delete process.env.VERCEL;
  Object.assign(process.env, { NODE_ENV: "test" });
  process.env.FILECHEAP_AUTH_SECRET = "s".repeat(32);
  process.env.FILECHEAP_OWNER_ACCOUNT_ID = "acc_owner123";
  process.env.FILECHEAP_OWNER_EMAIL = "Owner@Example.com";
  process.env.PLATFORM_PUBLIC_URL = "http://127.0.0.1:3100";
  process.env.RESEND_AUTH_FROM = "file.cheap <auth@example.com>";
  process.env.RESEND_AUTH_SEND_API_KEY = "re_test";
  expect(getAuthConfig()).toMatchObject({
    allowedEmails: ["owner@example.com"],
    ownerAccountId: "acc_owner123",
    publicUrl: "http://127.0.0.1:3100",
  });
});

test("fails closed without an owner, strong secret, sender, or production HTTPS", () => {
  delete process.env.VERCEL;
  Object.assign(process.env, { NODE_ENV: "production" });
  process.env.FILECHEAP_AUTH_SECRET = "short";
  process.env.FILECHEAP_OWNER_ACCOUNT_ID = "owner";
  process.env.FILECHEAP_OWNER_EMAIL = "invalid";
  process.env.PLATFORM_PUBLIC_URL = "http://file.cheap";
  process.env.RESEND_AUTH_FROM = "";
  process.env.RESEND_AUTH_SEND_API_KEY = "";
  expect(() => getAuthConfig()).toThrow();
});
