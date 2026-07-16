import { describe, expect, test } from "bun:test";
import { z } from "zod";

import {
  AcknowledgedMutationResponseError,
  parseSuccessResponse,
  readBoundedJsonResponse,
  ResponseContractError,
} from "@/features/sync/response-contract";

const responseSchema = z.object({ ok: z.literal(true) }).strict();

describe("success response contracts", () => {
  test("returns only a response that matches the runtime schema", async () => {
    await expect(
      parseSuccessResponse(
        Response.json({ ok: true }),
        responseSchema,
        { operation: "Plan request" },
      ),
    ).resolves.toEqual({ ok: true });
  });

  test("rejects malformed JSON and structurally invalid success bodies", async () => {
    for (const response of [
      new Response("{", { status: 200 }),
      Response.json({ ok: true, unexpected: true }),
      Response.json({ ok: false }),
    ]) {
      await expect(
        parseSuccessResponse(response, responseSchema, {
          operation: "Plan request",
        }),
      ).rejects.toBeInstanceOf(ResponseContractError);
    }
  });

  test("requires JSON media type and bounds streamed control-plane bodies", async () => {
    await expect(
      parseSuccessResponse(
        new Response(JSON.stringify({ ok: true }), {
          headers: { "content-type": "text/plain" },
        }),
        responseSchema,
        { operation: "Plan request" },
      ),
    ).rejects.toBeInstanceOf(ResponseContractError);

    await expect(
      readBoundedJsonResponse(
        new Response(JSON.stringify({ padding: "x".repeat(32) })),
        16,
      ),
    ).rejects.toThrow("byte limit");
    await expect(
      readBoundedJsonResponse(
        new Response("{}", { headers: { "content-length": "999" } }),
        16,
      ),
    ).rejects.toThrow("byte limit");

    let canceled = false;
    const body = new ReadableStream<Uint8Array>({
      cancel() {
        canceled = true;
      },
    });
    await expect(
      readBoundedJsonResponse(
        new Response(body, { headers: { "content-length": "999" } }),
        16,
      ),
    ).rejects.toThrow("byte limit");
    expect(canceled).toBe(true);
  });

  test("marks an invalid 2xx mutation response as acknowledged and keeps its request ID", async () => {
    const parseInvalidCommit = () =>
      parseSuccessResponse(
        Response.json(
          { committed: "maybe" },
          { headers: { "x-request-id": "commit-contract-01" } },
        ),
        responseSchema,
        {
          acknowledgedMutation: true,
          operation: "Commit",
        },
      );

    await expect(
      parseInvalidCommit(),
    ).rejects.toMatchObject({
      message:
        "Commit was acknowledged, but its success response was invalid. Do not upload again; reconnect to reconcile the catalog.",
      name: "AcknowledgedMutationResponseError",
      requestId: "commit-contract-01",
    });

    try {
      await parseInvalidCommit();
    } catch (error) {
      expect(error).toBeInstanceOf(AcknowledgedMutationResponseError);
    }
  });
});
