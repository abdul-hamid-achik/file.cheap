import { getConfig } from "@/shared/config/env";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export function GET(): Response {
  return Response.json({
    database: "none",
    deployment: "local-prototype",
    status: "ok",
    storage: getConfig().storageDriver,
    version: "filecheap-sync/1",
  });
}
