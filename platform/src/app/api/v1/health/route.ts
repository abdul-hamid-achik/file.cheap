import { getObjectStore } from "@/platform/storage/factory";
import { jsonResponse } from "@/shared/http/response";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export function GET(request: Request): Response {
  const store = getObjectStore();
  return jsonResponse(request, {
    database: "none",
    deployment: "local-prototype",
    status: "ok",
    storage: store.driver,
    storageVerification: store.verification,
    version: "filecheap-sync/1",
  });
}
