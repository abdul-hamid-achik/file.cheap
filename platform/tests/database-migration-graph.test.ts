import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";

import { maximumArtifactBytes } from "@/shared/config/limits";

type MigrationJournal = {
  entries: Array<{ idx: number; tag: string }>;
};

type MigrationSnapshot = {
  tables: Record<
    string,
    {
      checkConstraints: Record<string, { name: string; value: string }>;
    }
  >;
};

const drizzleDirectory = `${process.cwd()}/drizzle`;
const metadataDirectory = `${drizzleDirectory}/meta`;
const journal = JSON.parse(
  readFileSync(`${metadataDirectory}/_journal.json`, "utf8"),
) as MigrationJournal;

describe("database migration graph", () => {
  test("keeps SQL, journal entries, and snapshots one-to-one", () => {
    const entries = [...journal.entries].sort(
      (left, right) => left.idx - right.idx,
    );
    const sqlTags = readdirSync(drizzleDirectory)
      .filter((name) => name.endsWith(".sql"))
      .map((name) => name.slice(0, -4))
      .sort();
    const snapshotIndexes = readdirSync(metadataDirectory)
      .filter((name) => /^\d{4}_snapshot\.json$/.test(name))
      .map((name) => Number(name.slice(0, 4)))
      .sort((left, right) => left - right);

    expect(entries.map((entry) => entry.idx)).toEqual(
      entries.map((_, index) => index),
    );
    expect(entries.map((entry) => entry.tag)).toEqual(sqlTags);
    expect(snapshotIndexes).toEqual(entries.map((entry) => entry.idx));
  });

  test("records every private artifact integrity constraint", () => {
    const latest = [...journal.entries].sort(
      (left, right) => left.idx - right.idx,
    ).at(-1);
    if (!latest) {
      throw new Error("Expected at least one database migration");
    }
    const snapshot = JSON.parse(
      readFileSync(
        `${metadataDirectory}/${String(latest.idx).padStart(4, "0")}_snapshot.json`,
        "utf8",
      ),
    ) as MigrationSnapshot;

    expect(
      Object.keys(
        snapshot.tables["public.artifacts"]?.checkConstraints ?? {},
      ).sort(),
    ).toEqual([
      "artifacts_expiry_check",
      "artifacts_plan_receipt_shape_check",
      "artifacts_sha256_check",
      "artifacts_size_check",
      "artifacts_state_check",
      "artifacts_verification_check",
    ]);
    expect(
      Object.keys(
        snapshot.tables["public.artifact_objects"]?.checkConstraints ?? {},
      ).sort(),
    ).toEqual([
      "artifact_objects_ordinal_check",
      "artifact_objects_size_check",
    ]);

    // The Postgres ceiling is a literal in schema.ts because drizzle-kit reads
    // that file outside the Next.js path aliases. Assert it here so the SQL and
    // the runtime contract cannot drift apart.
    for (const [table, constraint] of [
      ["public.artifacts", "artifacts_size_check"],
      ["public.artifact_objects", "artifact_objects_size_check"],
    ] as const) {
      expect(
        snapshot.tables[table]?.checkConstraints?.[constraint]?.value,
      ).toContain(`<= ${maximumArtifactBytes}`);
    }
  });

  test("records the private retention and append-only activity invariants", () => {
    const latest = [...journal.entries].sort(
      (left, right) => left.idx - right.idx,
    ).at(-1);
    if (!latest) throw new Error("Expected a retention migration");
    const snapshot = JSON.parse(
      readFileSync(
        `${metadataDirectory}/${String(latest.idx).padStart(4, "0")}_snapshot.json`,
        "utf8",
      ),
    ) as MigrationSnapshot;

    expect(Object.keys(
      snapshot.tables["public.private_retention_runs"]?.checkConstraints ?? {},
    ).sort()).toEqual([
      "private_retention_runs_counters_check",
      "private_retention_runs_failed_areas_check",
      "private_retention_runs_id_check",
      "private_retention_runs_outcome_check",
      "private_retention_runs_status_check",
      "private_retention_runs_terminal_check",
      "private_retention_runs_time_check",
    ]);
    expect(Object.keys(
      snapshot.tables["public.private_activity_events"]?.checkConstraints ?? {},
    ).sort()).toEqual([
      "private_activity_events_actor_check",
      "private_activity_events_details_check",
      "private_activity_events_id_check",
      "private_activity_events_name_check",
      "private_activity_events_subject_check",
    ]);
    const migration = readFileSync(
      `${drizzleDirectory}/0012_private_retention_observability.sql`,
      "utf8",
    );
    expect(migration).toContain("private_retention_runs_one_running_unique");
    expect(migration).toContain("private_activity_events_append_only");
  });
});
