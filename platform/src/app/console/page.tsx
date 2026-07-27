import type { Metadata, Route } from "next";
import { redirect } from "next/navigation";

import { getConsoleCatalogService } from "@/features/console/catalog/factory";
import {
  artifactPageState,
  type ConsolePageSearchParams,
} from "@/features/console/catalog/page-params";
import { ArtifactDashboard } from "@/features/console/ui/ArtifactDashboard";
import { ConsoleShell } from "@/features/console/ui/ConsoleShell";
import { requireConsoleSession } from "@/shared/auth/console-session";
import { PlatformError } from "@/shared/errors/platform-error";

export const metadata: Metadata = { title: "Artifact console" };

export default async function ConsolePage({
  searchParams,
}: {
  searchParams: Promise<ConsolePageSearchParams>;
}) {
  let session: Awaited<ReturnType<typeof requireConsoleSession>>;
  try {
    session = await requireConsoleSession();
  } catch (error) {
    if (error instanceof PlatformError && error.code === "unauthorized") {
      redirect("/console/login" as Route);
    }
    throw error;
  }
  const state = artifactPageState(await searchParams);
  const catalog = await getConsoleCatalogService().listArtifacts(
    state.query,
    session.userId,
  );
  return (
    <ConsoleShell
      navigation={[
        { current: true, href: "/console", label: "Artifacts" },
        { href: "/console/runs", label: "Runs" },
        { href: "/console/access", label: "Access" },
      ]}
      sessionLabel={session.email}
    >
      <ArtifactDashboard
        catalog={catalog}
        groupBy={state.groupBy}
        page={state.page}
        query={state.query}
      />
    </ConsoleShell>
  );
}
