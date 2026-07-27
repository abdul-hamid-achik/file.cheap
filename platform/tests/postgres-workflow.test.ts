import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";

const repositoryRoot = `${process.cwd()}/..`;
const ciWorkflow = readFileSync(
  `${repositoryRoot}/.github/workflows/ci.yml`,
  "utf8",
);
const productionWorkflow = readFileSync(
  `${repositoryRoot}/.github/workflows/production-release.yml`,
  "utf8",
);
const packageManifest = JSON.parse(
  readFileSync(`${process.cwd()}/package.json`, "utf8"),
) as { scripts: Record<string, string> };

describe("PostgreSQL integration gate", () => {
  test("keeps the migration-from-zero runner available as a Bun script", () => {
    expect(packageManifest.scripts["test:postgres"]).toBe(
      "bun scripts/run-postgres-tests.ts",
    );
  });

  test("makes real PostgreSQL migrations mandatory in CI and release verification", () => {
    for (const workflow of [ciWorkflow, productionWorkflow]) {
      expect(workflow).toContain(
        "postgres:16-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777",
      );
      expect(workflow).toContain("FILECHEAP_POSTGRES_TEST_URL:");
      expect(workflow).toContain("bun run test:postgres");
    }
  });

  test("does not reuse migration or runtime production variables for tests", () => {
    const testSteps = [ciWorkflow, productionWorkflow]
      .map((workflow) =>
        workflow.match(
          /- name: Apply migrations from zero on PostgreSQL[\s\S]*?run: bun run test:postgres/,
        )?.[0],
      )
      .filter((step): step is string => Boolean(step));

    expect(testSteps).toHaveLength(2);
    for (const step of testSteps) {
      expect(step).not.toContain("MIGRATIONS_DATABASE_URL");
      expect(step).not.toContain("DATABASE_URL:");
    }
  });
});
