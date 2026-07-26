import { deviceRefreshInputSchema } from "@/features/auth/contracts";
import { getAuthService } from "@/features/auth/factory";
import { enforceConsoleRateLimit } from "@/platform/database/console-rate-limiter";
import { parseJson, parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export async function POST(request: Request): Promise<Response> {
  try {
    const input = parseRequest(deviceRefreshInputSchema, await parseJson(request));
    await enforceConsoleRateLimit({
      action: "device-refresh-ip",
      key: "request",
      limit: 30,
      request,
      windowSeconds: 15 * 60,
    });
    await enforceConsoleRateLimit({
      action: "device-refresh-token",
      includeAddress: false,
      key: input.refreshToken,
      limit: 10,
      request,
      windowSeconds: 60,
    });
    return jsonResponse(request, await getAuthService().refresh(input));
  } catch (error) {
    return problemResponse(error, request);
  }
}
