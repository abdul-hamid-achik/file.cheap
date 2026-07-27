import {
  accessDeviceRevokeResponseSchema,
  deviceFamilyIdSchema,
} from "@/features/auth/contracts";
import { getAuthService } from "@/features/auth/factory";
import { enforceConsoleRateLimit } from "@/platform/database/console-rate-limiter";
import { assertConsoleMutationOrigin } from "@/shared/auth/console-origin";
import { requireConsoleWebPrincipal } from "@/shared/auth/console-principal";
import { parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

type Context = { params: Promise<{ familyId: string }> };

export async function DELETE(request: Request, context: Context): Promise<Response> {
  try {
    assertConsoleMutationOrigin(request);
    const principal = await requireConsoleWebPrincipal(request);
    const { familyId: rawFamilyId } = await context.params;
    const familyId = parseRequest(deviceFamilyIdSchema, rawFamilyId);
    await enforceConsoleRateLimit({
      action: "access-device-revoke",
      includeAddress: false,
      key: principal.userId,
      limit: 20,
      request,
      windowSeconds: 60 * 60,
    });
    const result = await getAuthService().revokeAccessDevice(familyId, principal.userId);
    return jsonResponse(request, accessDeviceRevokeResponseSchema.parse(result));
  } catch (error) {
    return problemResponse(error, request);
  }
}
