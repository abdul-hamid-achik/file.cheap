import { Resend } from "resend";

import {
  type InboundMailClient,
  processInboundEmail,
} from "@/features/mail/inbound";
import type { InboundReplayRepository } from "@/features/mail/replay-repository";
import { DrizzleInboundReplayRepository } from "@/platform/database/inbound-email-replay-repository";
import { getEmailRuntimeConfig } from "@/platform/mail/config";
import { ResendReceivingClient } from "@/platform/mail/resend-receiving-client";
import { PlatformError } from "@/shared/errors/platform-error";
import {
  methodNotAllowedResponse,
  problemResponse,
} from "@/shared/http/problem";
import { readLimitedUtf8Body } from "@/shared/http/raw-body";
import { jsonResponse } from "@/shared/http/response";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

const MAX_WEBHOOK_BODY_BYTES = 64 * 1_024;
const MAX_HEADER_LENGTHS = {
  id: 256,
  signature: 2_048,
  timestamp: 64,
} as const;

interface ResendWebhookHeaders {
  id: string;
  signature: string;
  timestamp: string;
}

export interface ResendWebhookDependencies {
  client: InboundMailClient;
  forwardTo: string;
  replay: InboundReplayRepository;
  verify: (input: {
    payload: string;
    headers: ResendWebhookHeaders;
    webhookSecret: string;
  }) => unknown;
  webhookSecret: string;
}

function invalidWebhook(): PlatformError {
  return new PlatformError({
    code: "invalid_webhook",
    detail: "The webhook signature or headers are invalid.",
    status: 401,
    title: "Invalid webhook",
  });
}

function webhookHeaders(request: Request): ResendWebhookHeaders {
  const id = request.headers.get("svix-id") ?? "";
  const signature = request.headers.get("svix-signature") ?? "";
  const timestamp = request.headers.get("svix-timestamp") ?? "";
  if (
    !id ||
    !signature ||
    !timestamp ||
    id.length > MAX_HEADER_LENGTHS.id ||
    signature.length > MAX_HEADER_LENGTHS.signature ||
    timestamp.length > MAX_HEADER_LENGTHS.timestamp
  ) {
    throw invalidWebhook();
  }
  return { id, signature, timestamp };
}

export async function handleResendWebhook(
  request: Request,
  dependencies: ResendWebhookDependencies,
): Promise<Response> {
  try {
    const mediaType = request.headers
      .get("content-type")
      ?.split(";", 1)[0]
      ?.trim()
      .toLowerCase();
    if (mediaType !== "application/json") {
      throw new PlatformError({
        code: "unsupported_media_type",
        detail: "The webhook Content-Type must be application/json.",
        status: 415,
        title: "Unsupported media type",
      });
    }

    const payload = await readLimitedUtf8Body(
      request,
      MAX_WEBHOOK_BODY_BYTES,
    );
    const headers = webhookHeaders(request);
    let event: unknown;
    try {
      event = dependencies.verify({
        payload,
        headers,
        webhookSecret: dependencies.webhookSecret,
      });
    } catch {
      throw invalidWebhook();
    }

    const result = await processInboundEmail(event, {
      client: dependencies.client,
      forwardTo: dependencies.forwardTo,
      replay: dependencies.replay,
      svixId: headers.id,
    });
    if (result.action === "in_progress") {
      throw new PlatformError({
        code: "inbound_email_processing",
        detail: "Inbound email delivery is still being processed.",
        retryAfterSeconds: 15,
        status: 503,
        title: "Inbound email processing",
      });
    }
    return jsonResponse(request, { ok: true, ...result });
  } catch (error) {
    return problemResponse(error, request);
  }
}

export async function POST(request: Request): Promise<Response> {
  try {
    const config = getEmailRuntimeConfig();
    const resend = new Resend(config.receiveApiKey);
    return await handleResendWebhook(request, {
      client: new ResendReceivingClient(config.receiveApiKey),
      forwardTo: config.forwardTo,
      replay: new DrizzleInboundReplayRepository(),
      verify: (input) => resend.webhooks.verify(input),
      webhookSecret: config.webhookSecret,
    });
  } catch (error) {
    return problemResponse(error, request);
  }
}

function unsupportedMethod(request: Request): Response {
  return methodNotAllowedResponse(request, ["POST"]);
}

export {
  unsupportedMethod as DELETE,
  unsupportedMethod as GET,
  unsupportedMethod as HEAD,
  unsupportedMethod as OPTIONS,
  unsupportedMethod as PATCH,
  unsupportedMethod as PUT,
};
