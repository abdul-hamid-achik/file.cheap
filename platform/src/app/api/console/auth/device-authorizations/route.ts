import { deviceAuthorizationInputSchema } from "@/features/auth/contracts";
import { getAuthService } from "@/features/auth/factory";
import { parseJson, parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
import { enforceConsoleRateLimit } from "@/platform/database/console-rate-limiter";

export async function POST(request: Request): Promise<Response> {
  try {
    const input = parseRequest(deviceAuthorizationInputSchema, await parseJson(request));
    await enforceConsoleRateLimit({ action: "device-start", key: "request", limit: 5, request, windowSeconds: 15 * 60 });
    return jsonResponse(request, await getAuthService().startDeviceAuthorization(input), { status: 201 });
  } catch (error) {
    return problemResponse(error, request);
  }
}
