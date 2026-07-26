import { getAuthService } from "@/features/auth/factory";
import { PlatformError } from "@/shared/errors/platform-error";

export async function requireConsolePrincipal(request: Request) {
  const authorization = request.headers.get("authorization");
  if (authorization !== null) {
    const token = /^Bearer[\t ]+([^\s]+)$/iu.exec(authorization)?.[1];
    if (!token) throw unauthorized();
    return getAuthService().authenticate(token, "device");
  }
  const token = cookieValue(request.headers.get("cookie"), "__Host-fcheap_session")
    ?? cookieValue(request.headers.get("cookie"), "fcheap_session");
  if (!token) throw unauthorized();
  return getAuthService().authenticate(token, "web");
}

export async function requireConsoleWebPrincipal(request: Request) {
  if (request.headers.has("authorization")) throw unauthorized();
  const token = cookieValue(request.headers.get("cookie"), "__Host-fcheap_session")
    ?? cookieValue(request.headers.get("cookie"), "fcheap_session");
  if (!token) throw unauthorized();
  return getAuthService().authenticate(token, "web");
}

function cookieValue(header: string | null, name: string): string | undefined {
  return header?.split(";").map((part) => part.trim()).find((part) => part.startsWith(`${name}=`))?.slice(name.length + 1);
}

function unauthorized(): PlatformError {
  return new PlatformError({ code: "unauthorized", detail: "A valid console session is required.", status: 401, title: "Unauthorized" });
}
