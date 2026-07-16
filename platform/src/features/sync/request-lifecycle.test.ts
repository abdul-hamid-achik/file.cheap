import { describe, expect, test } from "bun:test";

import {
  OperationCanceledError,
  RequestTimeoutError,
  throwIfOperationCanceled,
  withRequestDeadline,
} from "@/features/sync/request-lifecycle";

describe("browser request lifecycle", () => {
  test("returns a completed operation and clears its deadline", async () => {
    const controller = new AbortController();

    await expect(
      withRequestDeadline({
        label: "Control-plane request",
        operation: async (signal) => {
          expect(signal.aborted).toBeFalse();
          return "complete";
        },
        signal: controller.signal,
        timeoutMilliseconds: 100,
      }),
    ).resolves.toBe("complete");
  });

  test("maps user cancellation to a stable recovery message", async () => {
    const controller = new AbortController();
    controller.abort();

    await expect(
      withRequestDeadline({
        label: "Archive upload",
        operation: async () => "unreachable",
        signal: controller.signal,
        timeoutMilliseconds: 100,
      }),
    ).rejects.toBeInstanceOf(OperationCanceledError);
  });

  test("aborts a request that exceeds its deadline", async () => {
    const controller = new AbortController();

    await expect(
      withRequestDeadline({
        label: "Recovery download",
        operation: (signal) =>
          new Promise((_, reject) => {
            signal.addEventListener("abort", () => reject(signal.reason), {
              once: true,
            });
          }),
        signal: controller.signal,
        timeoutMilliseconds: 5,
      }),
    ).rejects.toBeInstanceOf(RequestTimeoutError);
  });

  test("enforces the deadline when an operation ignores its signal", async () => {
    const controller = new AbortController();

    await expect(
      withRequestDeadline({
        label: "Uncooperative dependency",
        operation: () => new Promise(() => undefined),
        signal: controller.signal,
        timeoutMilliseconds: 5,
      }),
    ).rejects.toBeInstanceOf(RequestTimeoutError);
  });

  test("returns control when the user cancels an in-flight operation", async () => {
    const controller = new AbortController();
    const request = withRequestDeadline({
      label: "Archive upload",
      operation: () => new Promise(() => undefined),
      signal: controller.signal,
      timeoutMilliseconds: 1_000,
    });

    controller.abort();

    await expect(request).rejects.toBeInstanceOf(OperationCanceledError);
  });

  test("stops local work after cancellation is observed", () => {
    const controller = new AbortController();
    controller.abort(new OperationCanceledError());

    expect(() => throwIfOperationCanceled(controller.signal)).toThrow(
      OperationCanceledError,
    );
  });
});
