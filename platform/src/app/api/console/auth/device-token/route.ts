import { deviceTokenInputSchema } from "@/features/auth/contracts";
import { getAuthService } from "@/features/auth/factory";
import { parseJson, parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";
import { enforceConsoleRateLimit } from "@/platform/database/console-rate-limiter";

export async function POST(request: Request): Promise<Response> {
  try {
    const input = parseRequest(deviceTokenInputSchema, await parseJson(request));
    await enforceConsoleRateLimit({ action: "device-poll", key: input.deviceCode, limit: 12, request, windowSeconds: 60 });
    return jsonResponse(request, await getAuthService().poll(input.deviceCode));
  } catch (error) {
    return problemResponse(error, request);
  }
}
