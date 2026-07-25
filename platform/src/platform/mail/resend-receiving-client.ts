import { z } from "zod";

import {
  AcceptedProviderReceiptError,
  InboundContentRejectedError,
} from "@/features/mail/errors";
import type { ForwardMessage, InboundMailClient, ReceivedEmailMetadata } from "@/features/mail/inbound";
import { PlatformError } from "@/shared/errors/platform-error";

const RESEND_API_ORIGIN = "https://api.resend.com";
const REQUEST_TIMEOUT_MILLISECONDS = 10_000;
const MAX_METADATA_BYTES = 4 * 1_024 * 1_024;
const MAX_SEND_RESPONSE_BYTES = 64 * 1_024;
const MAX_HTML_BYTES = 2 * 1_024 * 1_024;
const MAX_TEXT_BYTES = 1 * 1_024 * 1_024;
const MAX_SUBJECT_BYTES = 512;

const addressSchema = z.string().min(3).max(512);
const boundedText = (maxBytes: number) => z.string().refine(
  (value) => Buffer.byteLength(value, "utf8") <= maxBytes,
);
const metadataSchema = z.object({
  attachments: z.array(z.object({
    content_disposition: z.string().nullable(),
    content_id: z.string().nullable(),
    content_type: z.string().min(1).max(255),
    filename: z.string().nullable(),
    id: z.string().min(1).max(256),
    size: z.number().int().nonnegative(),
  })),
  bcc: z.array(addressSchema).nullable(),
  cc: z.array(addressSchema).nullable(),
  created_at: z.string().datetime({ offset: true }),
  from: addressSchema,
  headers: z.record(z.string(), z.string()).nullable(),
  html: boundedText(MAX_HTML_BYTES).nullable(),
  id: z.string().min(1).max(256).regex(/^[A-Za-z0-9_-]+$/u),
  message_id: z.string().min(1).max(998),
  object: z.literal("email"),
  received_for: z.array(addressSchema).min(1).max(64),
  reply_to: z.array(addressSchema).max(10).nullable(),
  subject: boundedText(MAX_SUBJECT_BYTES).regex(/^[^\r\n]*$/u),
  text: boundedText(MAX_TEXT_BYTES).nullable(),
  to: z.array(addressSchema).max(64),
});

const sendResponseSchema = z.object({ id: z.string().min(1).max(256) });

type Fetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>;

export class ResendReceivingClient implements InboundMailClient {
  constructor(
    private readonly apiKey: string,
    private readonly fetcher: Fetcher = fetch,
  ) {}

  async getMetadata(emailId: string): Promise<ReceivedEmailMetadata> {
    const response = await this.request(
      `${RESEND_API_ORIGIN}/emails/receiving/${encodeURIComponent(emailId)}?html_format=cid`,
      {
        cache: "no-store",
        headers: {
          ...this.authorizationHeaders(),
          accept: "application/json",
          "user-agent": "file.cheap inbound forwarding/1.0",
        },
      },
    );
    const body = await readBoundedJson(response, MAX_METADATA_BYTES);
    const parsed = metadataSchema.safeParse(body);
    if (!parsed.success || parsed.data.id !== emailId) {
      throw new InboundContentRejectedError();
    }
    return {
      attachmentsPresent: parsed.data.attachments.length > 0,
      from: parsed.data.from,
      html: parsed.data.html ?? undefined,
      id: parsed.data.id,
      receivedFor: parsed.data.received_for,
      replyTo: parsed.data.reply_to ?? [],
      subject: parsed.data.subject,
      text: parsed.data.text ?? undefined,
      to: parsed.data.to,
    };
  }

  async send(message: ForwardMessage): Promise<void> {
    const payload = JSON.stringify({
      from: message.from,
      html: message.html,
      reply_to: message.replyTo,
      subject: message.subject,
      text: message.text,
      to: message.to,
    });
    const response = await this.request(`${RESEND_API_ORIGIN}/emails`, {
      body: payload,
      cache: "no-store",
      headers: {
        ...this.authorizationHeaders(),
        accept: "application/json",
        "content-type": "application/json",
        "idempotency-key": message.idempotencyKey,
        "user-agent": "file.cheap inbound forwarding/1.0",
      },
      method: "POST",
    });
    let body: unknown;
    try {
      body = await readBoundedJson(response, MAX_SEND_RESPONSE_BYTES);
    } catch {
      throw new AcceptedProviderReceiptError();
    }
    if (!sendResponseSchema.safeParse(body).success) {
      throw new AcceptedProviderReceiptError();
    }
  }

  private authorizationHeaders(): HeadersInit {
    return { authorization: `Bearer ${this.apiKey}` };
  }

  private async request(url: string, init: RequestInit): Promise<Response> {
    let response: Response;
    try {
      response = await this.fetcher(url, {
        ...init,
        redirect: "error",
        signal: AbortSignal.timeout(REQUEST_TIMEOUT_MILLISECONDS),
      });
    } catch {
      throw providerFailure();
    }
    if (!response.ok) {
      await response.body?.cancel().catch(() => undefined);
      throw providerFailure();
    }
    return response;
  }
}

async function readBoundedJson(response: Response, maxBytes: number): Promise<unknown> {
  let bytes: Uint8Array;
  try {
    bytes = await readBoundedBytes(response, maxBytes);
  } catch (error) {
    if (
      error instanceof InboundContentRejectedError ||
      error instanceof PlatformError
    ) throw error;
    throw providerFailure();
  }
  try {
    return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
  } catch {
    throw new InboundContentRejectedError();
  }
}

async function readBoundedBytes(response: Response, maxBytes: number): Promise<Uint8Array> {
  const advertisedLength = response.headers.get("content-length");
  if (advertisedLength !== null && Number(advertisedLength) > maxBytes) {
    await response.body?.cancel().catch(() => undefined);
    throw new InboundContentRejectedError();
  }
  if (!response.body) throw providerFailure();
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > maxBytes) {
        await reader.cancel().catch(() => undefined);
        throw new InboundContentRejectedError();
      }
      chunks.push(value);
    }
  } catch (error) {
    if (error instanceof InboundContentRejectedError) throw error;
    throw providerFailure();
  } finally {
    reader.releaseLock();
  }
  const result = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    result.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return result;
}

function providerFailure(): PlatformError {
  return new PlatformError({
    code: "email_forwarding_failed",
    detail: "Inbound email forwarding could not be completed.",
    retryAfterSeconds: 1,
    status: 502,
    title: "Email forwarding failed",
  });
}
