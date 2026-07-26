import type { Metadata, Route } from "next";
import { redirect } from "next/navigation";

import { getArtifactService } from "@/features/artifacts/factory";
import { ArtifactDashboard } from "@/features/console/ui/ArtifactDashboard";
import { ConsoleShell } from "@/features/console/ui/ConsoleShell";
import { requireConsoleSession } from "@/shared/auth/console-session";
import { PlatformError } from "@/shared/errors/platform-error";

export const metadata: Metadata = { title: "Artifact console" };

export default async function ConsolePage() {
  let session: Awaited<ReturnType<typeof requireConsoleSession>>;
  try {
    session = await requireConsoleSession();
  } catch (error) {
    if (error instanceof PlatformError && error.code === "unauthorized") {
      redirect("/console/login" as Route);
    }
    throw error;
  }
  const { artifacts } = await getArtifactService().list({ limit: 100 }, session.userId);
  return (
    <ConsoleShell
      navigation={[
        { current: true, href: "/console", label: "Artifacts" },
        { href: "/console/runs", label: "Runs" },
        { label: "Access" },
      ]}
      sessionLabel={session.email}
    >
      <ArtifactDashboard artifacts={artifacts} />
    </ConsoleShell>
  );
}
