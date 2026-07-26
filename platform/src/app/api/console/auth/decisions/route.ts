import { authorizationDecisionInputSchema } from "@/features/auth/contracts";
import { getAuthService } from "@/features/auth/factory";
import { setConsoleSession } from "@/shared/auth/console-session";
import { assertConsoleMutationOrigin } from "@/shared/auth/console-origin";
import { parseJson, parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
import { enforceConsoleRateLimit } from "@/platform/database/console-rate-limiter";

export async function POST(request: Request): Promise<Response> {
  try {
    assertConsoleMutationOrigin(request);
    const input = parseRequest(authorizationDecisionInputSchema, await parseJson(request));
    await enforceConsoleRateLimit({ action: "verification-decision", key: input.userCode, limit: 5, request, windowSeconds: 60 * 60 });
    const result = await getAuthService().decide(input);
    if (result) await setConsoleSession(result.sessionToken);
    return jsonResponse(request, { status: result ? "approved" : "denied" });
  } catch (error) {
    return problemResponse(error, request);
  }
}
