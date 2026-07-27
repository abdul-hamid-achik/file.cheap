import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";

import {
  ArtifactDeleteConfirmation,
  ArtifactDetail,
  artifactPullCommand,
} from "./ArtifactDetail";
import type { ConsoleArtifact } from "./artifact-types";

const artifact: ConsoleArtifact = {
  availability: "cloud-ready",
  contentType: "application/zstd",
  id: "art_0123456789abcdef",
  integrity: "server-sha256",
  kind: "run-archive",
  label: "Recorded run",
  producer: {
    nativeId: "run:one",
    tool: "glyphrun",
  },
  sha256: "a".repeat(64),
  state: "committed",
};

describe("artifact detail", () => {
  test("renders accessible copy controls for stable artifact identifiers", () => {
    const html = renderToStaticMarkup(<ArtifactDetail artifact={artifact} onClose={() => undefined} />);

    expect(html).toContain('aria-label="Copy Artifact ID"');
    expect(html).toContain('aria-label="Copy Native ID"');
    expect(html).toContain('aria-label="Copy SHA-256"');
    expect(html).toContain('aria-label="Copy verified CLI recovery command"');
    expect(html).toContain('aria-live="polite"');
    expect(html).toContain("Artifact bytes are never proxied or previewed");
    expect(html).toContain("Recover with the CLI");
    expect(html).toContain("Download in browser");
    expect(html).not.toContain("Download verified transfer");
  });

  test("does not offer remote recovery for bytes that are unavailable", () => {
    const html = renderToStaticMarkup(
      <ArtifactDetail
        artifact={{ ...artifact, availability: "unavailable" }}
        onClose={() => undefined}
      />,
    );

    expect(html).not.toContain("Recover with the CLI");
    expect(html).not.toContain("Download in browser");
    expect(html).toContain("not currently eligible for a private cloud transfer");
  });

  test("builds a shell-safe recovery command from the validated artifact ID", () => {
    expect(artifactPullCommand("art_0123456789abcdef")).toBe(
      "fcheap pull art_0123456789abcdef --output ./artifact-download.bin",
    );
  });

  test("describes permanent deletion without an eager alert announcement", () => {
    const html = renderToStaticMarkup(
      <ArtifactDeleteConfirmation
        artifactId={artifact.id}
        busy={false}
        deleting={false}
        onCancel={() => undefined}
        onConfirm={() => undefined}
      />,
    );

    expect(html).toContain('role="group"');
    expect(html).toContain('aria-label="Permanent artifact deletion"');
    expect(html).toContain('aria-describedby="artifact-delete-description"');
    expect(html).toContain('id="artifact-delete-description"');
    expect(html).not.toContain('role="alert"');
  });
});
