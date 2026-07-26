"use client";

import { ArtifactDashboardError } from "@/features/console/ui/ArtifactDashboardError";

export default function ConsoleError({ reset }: Readonly<{ error: Error & { digest?: string }; reset: () => void }>) {
  return <ArtifactDashboardError retry={reset} />;
}
