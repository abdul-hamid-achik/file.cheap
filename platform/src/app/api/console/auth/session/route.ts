import { requireConsolePrincipal } from "@/shared/auth/console-principal";
import { problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export async function GET(request: Request): Promise<Response> {
  try {
    const principal = await requireConsolePrincipal(request);
    return jsonResponse(request, { email: principal.email, userId: principal.userId });
  } catch (error) {
    return problemResponse(error, request);
  }
}
