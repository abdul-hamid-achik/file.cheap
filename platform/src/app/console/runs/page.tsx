import type { Metadata, Route } from "next";
import { redirect } from "next/navigation";

import { ConsoleShell } from "@/features/console/ui/ConsoleShell";
import { getConsoleCatalogService } from "@/features/console/catalog/factory";
import {
  runPageState,
  type ConsolePageSearchParams,
} from "@/features/console/catalog/page-params";
import { RunDashboard } from "@/features/runs/ui/RunDashboard";
import { requireConsoleSession } from "@/shared/auth/console-session";
import { PlatformError } from "@/shared/errors/platform-error";

export const metadata: Metadata = { title: "Run console" };

export default async function RunsPage({
  searchParams,
}: {
  searchParams: Promise<ConsolePageSearchParams>;
}) {
  let session: Awaited<ReturnType<typeof requireConsoleSession>>;
  try {
    session = await requireConsoleSession();
  } catch (error) {
    if (error instanceof PlatformError && error.code === "unauthorized") redirect("/console/login" as Route);
    throw error;
  }
  const state = runPageState(await searchParams);
  const catalog = await getConsoleCatalogService().listRuns(
    state.query,
    session.userId,
  );
  return (
    <ConsoleShell
      navigation={[
        { href: "/console", label: "Artifacts" },
        { current: true, href: "/console/runs", label: "Runs" },
        { href: "/console/access", label: "Access" },
      ]}
      sessionLabel={session.email}
    >
      <RunDashboard catalog={catalog} page={state.page} query={state.query} />
    </ConsoleShell>
  );
}
