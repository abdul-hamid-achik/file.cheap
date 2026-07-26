import { PlatformError } from "@/shared/errors/platform-error";

export function assertConsoleMutationOrigin(request: Request): void {
  // Device bearer requests are not ambient browser authority and therefore do
  // not need a CSRF origin check. Authentication still fails closed later.
  if (request.headers.has("authorization")) return;
  const origin = request.headers.get("origin");
  const fetchSite = request.headers.get("sec-fetch-site");
  if (
    !origin ||
    origin !== new URL(request.url).origin ||
    (fetchSite !== null && fetchSite !== "same-origin")
  ) {
    throw new PlatformError({
      code: "cross_origin_request",
      detail: "Console mutations require a same-origin browser request.",
      status: 403,
      title: "Cross-origin request rejected",
    });
  }
}
