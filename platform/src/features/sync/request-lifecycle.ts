export class OperationCanceledError extends Error {
  constructor() {
    super(
      "Operation canceled. Retry safely to reconcile remote state; no local file was deleted.",
    );
    this.name = "OperationCanceledError";
  }
}

export class RequestTimeoutError extends Error {
  constructor(label: string) {
    super(
      `${label} timed out. Retry safely to reconcile remote state; no local file was deleted.`,
    );
    this.name = "RequestTimeoutError";
  }
}

type RequestDeadlineOptions<T> = {
  label: string;
  operation: (signal: AbortSignal) => Promise<T>;
  signal: AbortSignal;
  timeoutMilliseconds: number;
};

export async function withRequestDeadline<T>({
  label,
  operation,
  signal,
  timeoutMilliseconds,
}: RequestDeadlineOptions<T>): Promise<T> {
  const requestController = new AbortController();
  const cancelFromOperation = () => {
    abortWith(requestController, new OperationCanceledError());
  };

  if (signal.aborted) {
    cancelFromOperation();
  } else {
    signal.addEventListener("abort", cancelFromOperation, { once: true });
  }

  const timeout = setTimeout(() => {
    abortWith(requestController, new RequestTimeoutError(label));
  }, timeoutMilliseconds);
  let rejectOnAbort: (() => void) | undefined;

  try {
    throwIfOperationCanceled(requestController.signal);
    const aborted = new Promise<never>((_, reject) => {
      rejectOnAbort = () => {
        reject(abortReason(requestController.signal));
      };
      if (requestController.signal.aborted) {
        rejectOnAbort();
      } else {
        requestController.signal.addEventListener("abort", rejectOnAbort, {
          once: true,
        });
      }
    });
    const result = operation(requestController.signal);
    return await Promise.race([result, aborted]);
  } catch (error) {
    if (
      requestController.signal.aborted &&
      requestController.signal.reason instanceof Error
    ) {
      throw requestController.signal.reason;
    }
    throw error;
  } finally {
    clearTimeout(timeout);
    signal.removeEventListener("abort", cancelFromOperation);
    if (rejectOnAbort) {
      requestController.signal.removeEventListener("abort", rejectOnAbort);
    }
  }
}

export function throwIfOperationCanceled(signal: AbortSignal): void {
  if (!signal.aborted) return;
  throw abortReason(signal);
}

function abortWith(controller: AbortController, reason: Error): void {
  if (!controller.signal.aborted) controller.abort(reason);
}

function abortReason(signal: AbortSignal): Error {
  return signal.reason instanceof Error
    ? signal.reason
    : new OperationCanceledError();
}
