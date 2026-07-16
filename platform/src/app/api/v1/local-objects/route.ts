import { getObjectStore } from "@/platform/storage/factory";
import {
  LocalObjectStore,
  localTransferTokenHeader,
} from "@/platform/storage/local-object-store";
import { PlatformError } from "@/shared/errors/platform-error";
import {
  methodNotAllowedResponse,
  problemResponse,
} from "@/shared/http/problem";
import {
  attachResponseMetadata,
  jsonResponse,
} from "@/shared/http/response";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function PUT(request: Request): Promise<Response> {
  try {
    const { key, token } = parameters(request);
    const store = localStore();
    return jsonResponse(request, await store.acceptUpload(request, key, token), {
      status: 201,
    });
  } catch (error) {
    return problemResponse(error, request);
  }
}

export async function GET(request: Request): Promise<Response> {
  try {
    const { key, token } = parameters(request);
    return attachResponseMetadata(
      request,
      await localStore().serveDownload(key, token, request.signal),
    );
  } catch (error) {
    return problemResponse(error, request);
  }
}

function unsupportedMethod(request: Request): Response {
  return methodNotAllowedResponse(request, ["GET", "HEAD", "PUT"]);
}

export {
  unsupportedMethod as DELETE,
  unsupportedMethod as OPTIONS,
  unsupportedMethod as PATCH,
  unsupportedMethod as POST,
};

function parameters(request: Request): { key: string; token: string } {
  const url = new URL(request.url);
  const key = url.searchParams.get("key");
  const token = request.headers.get(localTransferTokenHeader);
  if (!key || !token) {
    throw new PlatformError({
      code: "missing_grant",
      detail: `Both the key query parameter and ${localTransferTokenHeader} header are required.`,
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
