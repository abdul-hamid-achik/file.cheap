import { describe, expect, test } from "bun:test";

import { InboundContentRejectedError } from "@/features/mail/errors";
import { ResendReceivingClient } from "@/platform/mail/resend-receiving-client";

const apiKey = `re_${"a".repeat(40)}`;

function metadata(overrides: Record<string, unknown> = {}) {
  return {
    attachments: [],
    bcc: null,
    cc: null,
    created_at: "2026-07-25T00:00:00.000Z",
    from: "sender@example.test",
    headers: null,
    html: "<p>hello</p>",
    id: "received-email-1",
    message_id: "<fixture@example.test>",
    object: "email",
    reply_to: ["reply@example.test"],
    subject: "Fixture",
    text: "hello",
    to: ["hello@file.cheap"],
    ...overrides,
  };
}

describe("Resend receiving client", () => {
  test("uses direct authenticated metadata fetches with redirect errors and strict response validation", async () => {
    let request: RequestInit | undefined;
    const client = new ResendReceivingClient(apiKey, async (_url, init) => {
      request = init;
      return Response.json(metadata());
    });
    await expect(client.getMetadata("received-email-1")).resolves.toMatchObject({
      id: "received-email-1",
      receivedFor: ["hello@file.cheap"],
      replyTo: ["reply@example.test"],
    });
    expect(request).toMatchObject({ redirect: "error" });
    expect(request).toMatchObject({ cache: "no-store" });
    expect(new Headers(request?.headers).get("authorization")).toBe(`Bearer ${apiKey}`);
  });

  test("fails closed on oversized or malformed metadata without exposing provider data", async () => {
    const oversized = new ResendReceivingClient(apiKey, async () => new Response(
      "x".repeat(4 * 1_024 * 1_024 + 1),
    ));
    await expect(oversized.getMetadata("received-email-1")).rejects
      .toBeInstanceOf(InboundContentRejectedError);

    const malformed = new ResendReceivingClient(apiKey, async () =>
      Response.json(metadata({ id: "different-email" })),
    );
    await expect(malformed.getMetadata("received-email-1")).rejects
      .toBeInstanceOf(InboundContentRejectedError);
  });

  test("sends only the bounded rendered body and stable idempotency key", async () => {
    let request: RequestInit | undefined;
    const client = new ResendReceivingClient(apiKey, async (_url, init) => {
      request = init;
      return Response.json({ id: "outbound-1" });
    });
    await client.send({
      from: "file.cheap Inbox <inbox@file.cheap>",
      idempotencyKey: "inbound-forward/filecheap/digest",
      replyTo: "reply@example.test",
      subject: "Fixture",
      text: "hello",
      to: "owner@example.test",
    });
    expect(new Headers(request?.headers).get("idempotency-key")).toBe(
      "inbound-forward/filecheap/digest",
    );
    expect(new Headers(request?.headers).get("accept")).toBe("application/json");
    expect(new Headers(request?.headers).get("user-agent")).toBe(
      "file.cheap inbound forwarding/1.0",
    );
    expect(request).toMatchObject({ cache: "no-store" });
    expect(String(request?.body)).not.toContain("attachment");
  });
});
