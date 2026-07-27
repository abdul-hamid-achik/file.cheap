import {
  accessDeviceListQuerySchema,
  accessDeviceListResponseSchema,
} from "@/features/auth/contracts";
import { getAuthService } from "@/features/auth/factory";
import { requireConsoleWebPrincipal } from "@/shared/auth/console-principal";
import { parseRequest, problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export async function GET(request: Request): Promise<Response> {
  try {
    const principal = await requireConsoleWebPrincipal(request);
    const url = new URL(request.url);
    const query = parseRequest(accessDeviceListQuerySchema, {
      cursor: url.searchParams.get("cursor") ?? undefined,
      limit: url.searchParams.has("limit")
        ? Number(url.searchParams.get("limit"))
        : undefined,
    });
    const page = await getAuthService().listAccessDevices(
      principal.userId,
      query,
    );
    return jsonResponse(
      request,
      accessDeviceListResponseSchema.parse({
        ...page,
        version: "filecheap-access/1",
      }),
    );
  } catch (error) {
    return problemResponse(error, request);
  }
}
