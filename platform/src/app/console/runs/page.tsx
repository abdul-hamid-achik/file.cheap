import type { Metadata, Route } from "next";
import { redirect } from "next/navigation";

import { ConsoleShell } from "@/features/console/ui/ConsoleShell";
import { getRunService } from "@/features/runs/factory";
import { RunDashboard } from "@/features/runs/ui/RunDashboard";
import { requireConsoleSession } from "@/shared/auth/console-session";
import { PlatformError } from "@/shared/errors/platform-error";

export const metadata: Metadata = { title: "Run console" };

export default async function RunsPage() {
  let session: Awaited<ReturnType<typeof requireConsoleSession>>;
  try {
    session = await requireConsoleSession();
  } catch (error) {
    if (error instanceof PlatformError && error.code === "unauthorized") redirect("/console/login" as Route);
    throw error;
  }
  const { runs } = await getRunService().list({ limit: 100 }, session.userId);
  return (
    <ConsoleShell
      navigation={[
        { href: "/console", label: "Artifacts" },
        { current: true, href: "/console/runs", label: "Runs" },
        { label: "Access" },
      ]}
      sessionLabel={session.email}
    >
      <RunDashboard runs={runs} />
    </ConsoleShell>
  );
}
