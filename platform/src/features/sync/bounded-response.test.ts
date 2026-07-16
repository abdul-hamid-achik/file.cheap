import { describe, expect, test } from "bun:test";

import { readExactResponseBytes } from "@/features/sync/bounded-response";

describe("bounded recovery responses", () => {
  test("reads exactly the expected bytes with or without Content-Length", async () => {
    const expected = new Uint8Array([1, 2, 3, 4]);

    for (const headers of [undefined, { "content-length": "4" }]) {
      const bytes = await readExactResponseBytes(
        new Response(expected, { headers }),
        expected.byteLength,
      );
      expect(new Uint8Array(bytes)).toEqual(expected);
    }
  });

  test("rejects a contradictory or malformed Content-Length before reading", async () => {
    for (const contentLength of ["3", "4.0", "-1", "not-a-number"]) {
      await expect(
        readExactResponseBytes(
          new Response(new Uint8Array([1, 2, 3, 4]), {
            headers: { "content-length": contentLength },
          }),
          4,
        ),
      ).rejects.toThrow("Content-Length");
    }

    let canceled = false;
    const body = new ReadableStream<Uint8Array>({
      cancel() {
        canceled = true;
      },
    });
    await expect(
      readExactResponseBytes(
        new Response(body, { headers: { "content-length": "999" } }),
        4,
      ),
    ).rejects.toThrow("Content-Length");
    expect(canceled).toBe(true);
  });

  test("rejects overlong and truncated streams without unbounded buffering", async () => {
    await expect(
      readExactResponseBytes(
        new Response(new Uint8Array([1, 2, 3, 4, 5])),
        4,
      ),
    ).rejects.toThrow("exceeded");

    await expect(
      readExactResponseBytes(new Response(new Uint8Array([1, 2, 3])), 4),
    ).rejects.toThrow("ended before");
  });

  test("rejects invalid expectations and missing non-empty bodies", async () => {
    await expect(
      readExactResponseBytes(new Response(null), Number.NaN),
    ).rejects.toThrow("Expected download size");
    await expect(
      readExactResponseBytes(new Response(null), 1),
    ).rejects.toThrow("did not include a body");
    await expect(
      readExactResponseBytes(new Response(null), 0),
    ).resolves.toEqual(new ArrayBuffer(0));
  });
});
