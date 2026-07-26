import { verificationEmailInputSchema } from "@/features/auth/contracts";
import { getAuthService } from "@/features/auth/factory";
import { assertConsoleMutationOrigin } from "@/shared/auth/console-origin";
import { parseJson, parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
import { enforceConsoleRateLimit } from "@/platform/database/console-rate-limiter";

export async function POST(request: Request): Promise<Response> {
  try {
    // Keep allowlist misses from returning immediately while a real Resend
    // request is still in flight. Durable outbox delivery remains the next
    // step for fully decoupling provider latency.
    const responseFloor = new Promise<void>((resolve) => setTimeout(resolve, 700));
    assertConsoleMutationOrigin(request);
    const input = parseRequest(verificationEmailInputSchema, await parseJson(request));
    await enforceConsoleRateLimit({ action: "verification-email-ip", key: "request", limit: 5, request, windowSeconds: 15 * 60 });
    await enforceConsoleRateLimit({ action: "verification-email-authorization", includeAddress: false, key: input.userCode.replace(/-/gu, "").toUpperCase(), limit: 3, request, windowSeconds: 15 * 60 });
    await enforceConsoleRateLimit({ action: "verification-email-account", includeAddress: false, key: input.email.trim().toLowerCase(), limit: 3, request, windowSeconds: 15 * 60 });
    await Promise.all([getAuthService().sendVerification(input), responseFloor]);
    return jsonResponse(request, { status: "verification_requested" }, { status: 202 });
  } catch (error) {
    return problemResponse(error, request);
  }
}
