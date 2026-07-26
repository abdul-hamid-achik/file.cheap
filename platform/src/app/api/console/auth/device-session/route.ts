import { getAuthService } from "@/features/auth/factory";
import { requireConsolePrincipal } from "@/shared/auth/console-principal";
import { problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export async function DELETE(request: Request): Promise<Response> {
  try {
    await requireConsolePrincipal(request);
    const token = /^Bearer[\t ]+([^\s]+)$/iu.exec(request.headers.get("authorization") ?? "")?.[1];
    if (!token) throw new Error("Authenticated device request did not contain a bearer token");
    await getAuthService().logout(token);
    return jsonResponse(request, { status: "signed_out" });
  } catch (error) {
    return problemResponse(error, request);
  }
}
