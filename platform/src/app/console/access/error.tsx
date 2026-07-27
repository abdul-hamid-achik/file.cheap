"use client";

import { AccessDashboardError } from "@/features/console/access/AccessDashboardError";

export default function AccessError({ reset }: Readonly<{ error: Error & { digest?: string }; reset: () => void }>) {
  return <AccessDashboardError retry={reset} />;
}
