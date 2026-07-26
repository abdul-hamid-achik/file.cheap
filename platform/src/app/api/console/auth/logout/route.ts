import { clearConsoleSession } from "@/shared/auth/console-session";
import { assertConsoleMutationOrigin } from "@/shared/auth/console-origin";
import { getAuthService } from "@/features/auth/factory";
import { problemResponse } from "@/shared/http/problem";
import { jsonResponse } from "@/shared/http/response";

export async function POST(request: Request): Promise<Response> {
  try {
    assertConsoleMutationOrigin(request);
    const token = await clearConsoleSession();
    if (token) await getAuthService().logout(token);
    return jsonResponse(request, { status: "signed_out" });
  } catch (error) {
    return problemResponse(error, request);
  }
}
