import { getObjectStore } from "@/platform/storage/factory";
import { LocalObjectStore } from "@/platform/storage/local-object-store";
import { PlatformError } from "@/shared/errors/platform-error";
import { problemResponse } from "@/shared/http/problem";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function PUT(request: Request): Promise<Response> {
  try {
    const { key, token } = parameters(request);
    const store = localStore();
    return Response.json(await store.acceptUpload(request, key, token), { status: 201 });
  } catch (error) {
    return problemResponse(error, request);
  }
}

export async function GET(request: Request): Promise<Response> {
  try {
    const { key, token } = parameters(request);
    return await localStore().serveDownload(key, token);
  } catch (error) {
    return problemResponse(error, request);
  }
}

function parameters(request: Request): { key: string; token: string } {
  const url = new URL(request.url);
  const key = url.searchParams.get("key");
  const token = url.searchParams.get("token");
  if (!key || !token) {
    throw new PlatformError({
      code: "missing_grant",
      detail: "Both key and signed transfer token are required.",
      status: 400,
      title: "Missing grant",
    });
  }
  return { key, token };
}

function localStore(): LocalObjectStore {
  const store = getObjectStore();
  if (!(store instanceof LocalObjectStore)) {
    throw new PlatformError({
      code: "route_unavailable",
      detail: "Local object transfer routes are disabled for this storage driver.",
      status: 404,
      title: "Route unavailable",
    });
  }
  return store;
}
