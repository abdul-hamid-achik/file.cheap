import type { Metadata, Route } from "next";
import { notFound, redirect } from "next/navigation";

import { artifactIdSchema } from "@/features/artifacts/contracts";
import { consoleArtifactRedirect } from "@/features/artifacts/web-url";
import { getConsoleCatalogService } from "@/features/console/catalog/factory";
import { requireConsoleSession } from "@/shared/auth/console-session";
import { PlatformError } from "@/shared/errors/platform-error";

export const metadata: Metadata = { robots: { follow: false, index: false }, title: "Open artifact" };

/**
 * Target of the `web_url` emitted on cloud ArtifactRefV1 documents. It is a
 * path-only route (the contract forbids queries), so it resolves the artifact
 * after authentication and redirects into the matching catalog drawer.
 */
export default async function ArtifactLinkPage({ params }: { params: Promise<{ artifactId: string }> }) {
  const parsed = artifactIdSchema.safeParse((await params).artifactId);
  if (!parsed.success) notFound();
  let session: Awaited<ReturnType<typeof requireConsoleSession>>;
  try {
    session = await requireConsoleSession();
  } catch (error) {
    if (error instanceof PlatformError && error.code === "unauthorized") redirect("/console/login" as Route);
    throw error;
  }
  const runs = await getConsoleCatalogService().listRuns(
    { direction: "next", limit: 50, q: parsed.data },
    session.userId,
  );
  const isIndexedRun = runs.runs.some((run) => run.artifactId === parsed.data);
  redirect(consoleArtifactRedirect(parsed.data, isIndexedRun) as Route);
}
