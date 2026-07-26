import { describe, expect, test } from "bun:test";
import { createHmac } from "node:crypto";
import { Resend } from "resend";

import {
  GET,
  handleResendWebhook,
  type ResendWebhookDependencies,
} from "@/app/api/webhooks/resend/route";
import type { InboundMailClient } from "@/features/mail/inbound";
import {
  InMemoryInboundReplayRepository,
  replayDigest,
} from "@/features/mail/replay-repository";

function mailClient(overrides: Partial<InboundMailClient> = {}): InboundMailClient {
  return {
    getMetadata: async (id) => ({
      attachmentsPresent: false,
      from: "sender@example.test",
      id,
      receivedFor: ["hello@file.cheap"],
      replyTo: [],
      subject: "Fixture",
      text: "Fixture body",
      to: ["hello@file.cheap"],
    }),
    send: async () => undefined,
    ...overrides,
  };
}

function dependencies(
  overrides: Partial<ResendWebhookDependencies> = {},
): ResendWebhookDependencies {
  return {
    client: mailClient(),
    forwardTo: "owner@example.test",
    replay: new InMemoryInboundReplayRepository(),
    verify: () => ({
      type: "email.received",
      data: {
        email_id: "received-email-1",
        from: "sender@example.test",
        to: ["hello@file.cheap"],
      },
    }),
    webhookSecret: `whsec_${"b".repeat(40)}`,
    ...overrides,
  };
}

function webhookRequest(body = "{}") {
  return new Request("https://file.cheap/api/webhooks/resend", {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "svix-id": "msg_fixture_1",
      "svix-signature": "v1,signature",
      "svix-timestamp": String(Math.floor(Date.now() / 1_000)),
    },
    body,
  });
}

describe("Resend webhook route", () => {
  test("verifies the untouched body before forwarding", async () => {
    let verifiedPayload = "";
    const response = await handleResendWebhook(
      webhookRequest('{ "signed": true }\n'),
      dependencies({
        verify: (input) => {
          verifiedPayload = input.payload;
          return {
            type: "email.received",
            data: {
              email_id: "received-email-1",
              from: "sender@example.test",
              to: ["hello@file.cheap"],
            },
          };
        },
      }),
    );

    expect(response.status).toBe(200);
    expect(verifiedPayload).toBe('{ "signed": true }\n');
    expect(await response.json()).toEqual({ ok: true, action: "forwarded" });
  });

  test("accepts a real Resend Svix signature fixture", async () => {
    const payload = JSON.stringify({
      type: "email.received",
      data: {
        email_id: "received-email-1",
        from: "sender@example.test",
        to: ["hello@file.cheap"],
      },
    });
    const id = "msg_fixture_real_signature";
    const timestamp = String(Math.floor(Date.now() / 1_000));
    const key = Buffer.from("resend fixture signing key", "utf8");
    const secret = `whsec_${key.toString("base64")}`;
    const signature = createHmac("sha256", key)
      .update(`${id}.${timestamp}.${payload}`, "utf8")
      .digest("base64");
    const request = new Request("https://file.cheap/api/webhooks/resend", {
      method: "POST",
      headers: {
        "content-type": "application/json",
        "svix-id": id,
        "svix-signature": `v1,${signature}`,
        "svix-timestamp": timestamp,
      },
      body: payload,
    });
    const resend = new Resend(`re_${"a".repeat(40)}`);
    const response = await handleResendWebhook(request, dependencies({
      verify: (input) => resend.webhooks.verify(input),
      webhookSecret: secret,
    }));
    expect(response.status).toBe(200);
  });

  test("rejects invalid signatures without fetching or forwarding", async () => {
    let used = false;
    const response = await handleResendWebhook(
      webhookRequest(),
      dependencies({
        client: mailClient({
          getMetadata: async () => {
            used = true;
            throw new Error("must not fetch");
          },
        }),
        verify: () => {
          throw new Error("invalid signature");
        },
      }),
    );
    expect(response.status).toBe(401);
    expect(used).toBe(false);
    expect(JSON.stringify(await response.json())).not.toContain("invalid signature");
  });

  test("rejects missing signature headers and unsupported media", async () => {
    const missingHeaders = await handleResendWebhook(
      new Request("https://file.cheap/api/webhooks/resend", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: "{}",
      }),
      dependencies(),
    );
    expect(missingHeaders.status).toBe(401);

    const wrongMedia = await handleResendWebhook(
      new Request("https://file.cheap/api/webhooks/resend", {
        method: "POST",
        headers: { "content-type": "text/plain" },
        body: "{}",
      }),
      dependencies(),
    );
    expect(wrongMedia.status).toBe(415);
  });

  test("bounds webhook bodies and rejects unsupported methods", async () => {
    const oversizedResponse = await handleResendWebhook(
      webhookRequest("x".repeat(64 * 1_024 + 1)),
      dependencies(),
    );
    expect(oversizedResponse.status).toBe(413);

    const methodRequest = new Request(
      "https://file.cheap/api/webhooks/resend",
      { method: "GET" },
    );
    const methodResponse = GET(methodRequest);
    expect(methodResponse.status).toBe(405);
    expect(methodResponse.headers.get("allow")).toBe("POST");
  });

  test("returns retryable 503 while the matching email lease is active", async () => {
    const activeReplay = new InMemoryInboundReplayRepository();
    await activeReplay.claim({
      emailIdSha256: replayDigest("received-email-1"),
      now: new Date(),
      svixIdSha256: replayDigest("msg_fixture_1"),
    });
    const response = await handleResendWebhook(webhookRequest(), dependencies({
      replay: activeReplay,
      verify: () => ({
        type: "email.received",
        data: {
          email_id: "received-email-1",
          from: "sender@example.test",
          to: ["hello@file.cheap"],
        },
      }),
    }));
    expect(response.status).toBe(503);
    expect(response.headers.get("retry-after")).toBe("15");
  });
});
