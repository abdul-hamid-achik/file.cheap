import { describe, expect, test } from "bun:test";

import { FILECHEAP_INBOUND_EMAIL } from "@/features/mail/inbound";
import { getEmailRuntimeConfig } from "@/platform/mail/config";
import { PlatformError } from "@/shared/errors/platform-error";

const validEnvironment = {
  RESEND_RECEIVE_API_KEY: `re_${"a".repeat(40)}`,
  RESEND_WEBHOOK_SECRET: `whsec_${"b".repeat(40)}`,
  RESEND_FORWARD_TO: "owner@example.test",
};

describe("email runtime configuration", () => {
  test("loads email credentials independently from the artifact service", () => {
    expect(getEmailRuntimeConfig(validEnvironment)).toMatchObject({
      forwardTo: "owner@example.test",
    });
    expect(FILECHEAP_INBOUND_EMAIL).toBe("hello@file.cheap");
  });

  test("fails closed for missing, malformed, or looping values", () => {
    expect(() => getEmailRuntimeConfig({})).toThrow(PlatformError);
    expect(() =>
      getEmailRuntimeConfig({
        ...validEnvironment,
        RESEND_WEBHOOK_SECRET: "invalid",
      }),
    ).toThrow(PlatformError);
    expect(() =>
      getEmailRuntimeConfig({
        ...validEnvironment,
        RESEND_FORWARD_TO: FILECHEAP_INBOUND_EMAIL,
      }),
    ).toThrow(PlatformError);
  });
});
