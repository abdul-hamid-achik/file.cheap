import { after } from "next/server";

import { verificationEmailInputSchema } from "@/features/auth/contracts";
import type { VerificationEmailInput } from "@/features/auth/contracts";
import type { PreparedVerificationDelivery } from "@/features/auth/service";
import { getAuthService } from "@/features/auth/factory";
import { assertConsoleMutationOrigin } from "@/shared/auth/console-origin";
import { parseJson, parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
import { enforceConsoleRateLimit } from "@/platform/database/console-rate-limiter";

type RateLimitInput = Parameters<typeof enforceConsoleRateLimit>[0];

export type VerificationEmailRouteDependencies = {
  defer(task: () => Promise<void>): void;
  dispatch(prepared: PreparedVerificationDelivery): Promise<void>;
  enforceRateLimit(input: RateLimitInput): Promise<void>;
  prepare(
    input: VerificationEmailInput,
  ): Promise<PreparedVerificationDelivery | null>;
  responseFloorMs: number;
};

const productionDependencies: VerificationEmailRouteDependencies = {
  defer: (task) => after(task),
  dispatch: (prepared) => getAuthService().dispatchVerification(prepared),
  enforceRateLimit: enforceConsoleRateLimit,
  prepare: (input) => getAuthService().prepareVerification(input),
  responseFloorMs: 700,
};

export async function handleVerificationEmailRequest(
  request: Request,
  dependencies: VerificationEmailRouteDependencies,
): Promise<Response> {
  try {
    // Provider work runs after the response and always gets the same floor.
    // Valid-but-unknown codes, allowlist misses, coalesced requests and Resend
    // failures therefore share one public 202 response and timing envelope.
    const responseFloor = new Promise<void>((resolve) =>
      setTimeout(resolve, dependencies.responseFloorMs)
    );
    assertConsoleMutationOrigin(request);
    const input = parseRequest(verificationEmailInputSchema, await parseJson(request));
    await dependencies.enforceRateLimit({
      action: "verification-email-ip",
      key: "request",
      limit: 12,
      request,
      windowSeconds: 15 * 60,
    });
    // The repository performs the same lookup for an unknown code or an
    // allowlist miss, but only an eligible request persists a leased claim.
    // A successful claim is therefore durable before the public 202 response.
    const prepared = await dependencies.prepare(input);
    if (prepared) {
      dependencies.defer(async () => {
        // Deferred work receives only the opaque claim. Provider failures and
        // fencing outcomes remain private and never log sensitive context.
        await dependencies.dispatch(prepared).catch(() => undefined);
      });
    }
    await responseFloor;
    return jsonResponse(request, { status: "verification_requested" }, { status: 202 });
  } catch (error) {
    return problemResponse(error, request);
  }
}

export async function POST(request: Request): Promise<Response> {
  return handleVerificationEmailRequest(request, productionDependencies);
}
