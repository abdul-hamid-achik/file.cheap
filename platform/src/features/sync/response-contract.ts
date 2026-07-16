import type { ZodType } from "zod";

export const controlPlaneResponseLimitBytes = 1024 * 1024;

export type SuccessResponseOptions = {
  acknowledgedMutation?: boolean;
  operation: string;
};

export class ResponseContractError extends Error {
  constructor(
    operation: string,
    readonly requestId?: string,
  ) {
    super(
      `${operation} returned an invalid success response. Reconnect before retrying; no local file was deleted.`,
    );
    this.name = "ResponseContractError";
  }
}

export class AcknowledgedMutationResponseError extends ResponseContractError {
  constructor(operation: string, requestId?: string) {
    super(operation, requestId);
    this.name = "AcknowledgedMutationResponseError";
    this.message = `${operation} was acknowledged, but its success response was invalid. Do not upload again; reconnect to reconcile the catalog.`;
  }
}

export class MutationOutcomeUnknownError extends Error {
  constructor(readonly requestId?: string) {
    super(
      "The commit request was dispatched, but its remote outcome is unknown. Do not upload again; reconnect to reconcile the catalog.",
    );
    this.name = "MutationOutcomeUnknownError";
  }
}

export async function parseSuccessResponse<T>(
  response: Response,
  schema: ZodType<T>,
  options: SuccessResponseOptions,
): Promise<T> {
  let input: unknown;
  try {
    const mediaType = response.headers
      .get("content-type")
      ?.split(";", 1)[0]
      ?.trim()
      .toLowerCase();
    if (mediaType !== "application/json") {
      throw new Error("Unexpected success response media type");
    }
    input = await readBoundedJsonResponse(
      response,
      controlPlaneResponseLimitBytes,
    );
  } catch {
    await cancelResponseBody(response, "Invalid control-plane response");
    throw responseContractError(response, options);
  }

  const parsed = schema.safeParse(input);
  if (!parsed.success) {
    throw responseContractError(response, options);
  }
  return parsed.data;
}

export async function readBoundedJsonResponse(
  response: Response,
  maximumBytes: number,
): Promise<unknown> {
  const advertisedLength = response.headers.get("content-length");
  if (advertisedLength !== null) {
    const parsedLength = Number(advertisedLength);
    if (
      !/^\d+$/.test(advertisedLength) ||
      !Number.isSafeInteger(parsedLength) ||
      parsedLength > maximumBytes
    ) {
      await cancelResponseBody(response, "Response JSON exceeded its byte limit");
      throw new Error("Response JSON exceeded its byte limit");
    }
  }

  if (!response.body) throw new Error("Response JSON body is missing");
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let receivedBytes = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      receivedBytes += value.byteLength;
      if (receivedBytes > maximumBytes) {
        await reader.cancel("Response JSON exceeded its byte limit").catch(
          () => undefined,
        );
        throw new Error("Response JSON exceeded its byte limit");
      }
      chunks.push(value);
    }
  } finally {
    reader.releaseLock();
  }

  const bytes = new Uint8Array(receivedBytes);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return JSON.parse(new TextDecoder("utf-8", { fatal: true }).decode(bytes));
}

async function cancelResponseBody(
  response: Response,
  reason: string,
): Promise<void> {
  if (!response.body || response.body.locked) return;
  await response.body.cancel(reason).catch(() => undefined);
}

function responseContractError(
  response: Response,
  options: SuccessResponseOptions,
): ResponseContractError {
  const requestId = response.headers.get("x-request-id") ?? undefined;
  return options.acknowledgedMutation
    ? new AcknowledgedMutationResponseError(options.operation, requestId)
    : new ResponseContractError(options.operation, requestId);
}
