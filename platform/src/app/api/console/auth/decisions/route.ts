import { authorizationDecisionInputSchema } from "@/features/auth/contracts";
import { getAuthService } from "@/features/auth/factory";
import { normalizeUserCode } from "@/features/auth/service";
import type { AuthorizationDecisionInput } from "@/features/auth/contracts";
import { setConsoleSession } from "@/shared/auth/console-session";
import { assertConsoleMutationOrigin } from "@/shared/auth/console-origin";
import { parseJson, parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
import { enforceConsoleRateLimit } from "@/platform/database/console-rate-limiter";

type DecisionResult = Awaited<ReturnType<ReturnType<typeof getAuthService>["decide"]>>;
type RateLimitInput = Parameters<typeof enforceConsoleRateLimit>[0];

export type AuthorizationDecisionRouteDependencies = {
  decide(input: AuthorizationDecisionInput): Promise<DecisionResult>;
  enforceRateLimit(input: RateLimitInput): Promise<void>;
  setSession(token: string): Promise<void>;
};

const productionDependencies: AuthorizationDecisionRouteDependencies = {
  decide: (input) => getAuthService().decide(input),
  enforceRateLimit: enforceConsoleRateLimit,
  setSession: setConsoleSession,
};

export async function handleAuthorizationDecisionRequest(
  request: Request,
  dependencies: AuthorizationDecisionRouteDependencies,
): Promise<Response> {
  try {
    assertConsoleMutationOrigin(request);
    const input = parseRequest(authorizationDecisionInputSchema, await parseJson(request));
    await dependencies.enforceRateLimit({
      action: "verification-decision",
      key: normalizeUserCode(input.userCode),
      limit: 5,
      request,
      windowSeconds: 60 * 60,
    });
    const result = await dependencies.decide(input);
    if (result?.sessionToken) await dependencies.setSession(result.sessionToken);
    return jsonResponse(request, {
      browserSession: result?.browserSession ?? false,
      status: result ? "approved" : "denied",
    });
  } catch (error) {
    return problemResponse(error, request);
  }
}

export async function POST(request: Request): Promise<Response> {
  return handleAuthorizationDecisionRequest(request, productionDependencies);
}
