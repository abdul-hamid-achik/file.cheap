import { describe, expect, test } from "bun:test";

import {
  artifactAvailabilityLabel,
  artifactIntegrityLabel,
  artifactStateLabel,
  formatArtifactBytes,
  groupConsoleArtifacts,
} from "./artifact-types";

describe("console artifact presentation", () => {
  test("formats byte counts without claiming unavailable sizes are zero", () => {
    expect(formatArtifactBytes(1_572_864)).toBe("1.50 MiB");
    expect(formatArtifactBytes(undefined)).toBe("Size unavailable");
  });

  test("uses explicit evidence labels for each artifact state", () => {
    expect(artifactStateLabel("committed")).toBe("Committed");
    expect(artifactAvailabilityLabel("local-vault-required")).toBe(
      "Local vault required",
    );
    expect(artifactIntegrityLabel("declared")).toBe(
      "Checksum declared by producer",
    );
  });

  test("groups the already-authorized artifact rows without changing them", () => {
    const groups = groupConsoleArtifacts([
      { availability: "cloud-ready", id: "art_1", integrity: "server-sha256", kind: "trace", label: "run-a", producer: { tool: "glyphrun" }, state: "committed" },
      { availability: "cloud-ready", id: "art_2", integrity: "server-sha256", kind: "report", label: "run-b", producer: { tool: "cairntrace" }, state: "committed" },
      { availability: "cloud-ready", id: "art_3", integrity: "server-sha256", kind: "trace", label: "run-c", producer: { tool: "glyphrun" }, state: "committed" },
    ], "producer");
    expect(groups.map((group) => [group.label, group.artifacts.length])).toEqual([
      ["cairntrace", 1],
      ["glyphrun", 2],
    ]);
  });
});
