import type { Metadata, Route } from "next";
import { redirect } from "next/navigation";

import { accessDeviceListQuerySchema } from "@/features/auth/contracts";
import { getAuthService } from "@/features/auth/factory";
import { AccessDashboard } from "@/features/console/access/AccessDashboard";
import { ConsoleShell } from "@/features/console/ui/ConsoleShell";
import { requireConsoleSession } from "@/shared/auth/console-session";
import { PlatformError } from "@/shared/errors/platform-error";

export const metadata: Metadata = { title: "Access console" };

export default async function AccessPage() {
  let session: Awaited<ReturnType<typeof requireConsoleSession>>;
  try {
    session = await requireConsoleSession();
  } catch (error) {
    if (error instanceof PlatformError && error.code === "unauthorized") redirect("/console/login" as Route);
    throw error;
  }
  const query = accessDeviceListQuerySchema.parse({});
  const page = await getAuthService().listAccessDevices(session.userId, query);
  return (
    <ConsoleShell
      navigation={[
        { href: "/console", label: "Artifacts" },
        { href: "/console/runs", label: "Runs" },
        { current: true, href: "/console/access", label: "Access" },
      ]}
      sessionLabel={session.email}
    >
      <AccessDashboard initialPage={page} />
    </ConsoleShell>
  );
}
