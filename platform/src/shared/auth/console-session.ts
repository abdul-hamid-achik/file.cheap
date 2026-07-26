import { cookies } from "next/headers";

import { getAuthService } from "@/features/auth/factory";
import { PlatformError } from "@/shared/errors/platform-error";

export const consoleSessionCookie = "__Host-fcheap_session";
const localConsoleSessionCookie = "fcheap_session";

export async function requireConsoleSession() {
  const store = await cookies();
  const token = store.get(cookieName())?.value;
  if (!token) throw unauthorized();
  return getAuthService().authenticate(token, "web");
}

export async function setConsoleSession(token: string): Promise<void> {
  const store = await cookies();
  store.set(cookieName(), token, {
    httpOnly: true,
    maxAge: 8 * 60 * 60,
    path: "/",
    sameSite: "lax",
    secure: isSecureDeployment(),
  });
}

export async function clearConsoleSession(): Promise<string | undefined> {
  const store = await cookies();
  const name = cookieName();
  const token = store.get(name)?.value;
  store.set(name, "", {
    httpOnly: true,
    maxAge: 0,
    path: "/",
    sameSite: "lax",
    secure: isSecureDeployment(),
  });
  return token;
}

function cookieName(): string {
  return isSecureDeployment() ? consoleSessionCookie : localConsoleSessionCookie;
}

function isSecureDeployment(): boolean {
  return process.env.NODE_ENV === "production" || Boolean(process.env.VERCEL);
}

function unauthorized(): PlatformError {
  return new PlatformError({ code: "unauthorized", detail: "A valid console session is required.", status: 401, title: "Unauthorized" });
}
