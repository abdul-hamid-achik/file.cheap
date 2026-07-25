import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";

const repositoryRoot = `${process.cwd()}/..`;
const workflow = readFileSync(
  `${repositoryRoot}/.github/workflows/production-release.yml`,
  "utf8",
);
const vercelConfig = JSON.parse(
  readFileSync(`${process.cwd()}/vercel.json`, "utf8"),
) as {
  git?: { deploymentEnabled?: boolean | Record<string, boolean> };
};
const migrationJournal = JSON.parse(
  readFileSync(`${process.cwd()}/drizzle/meta/_journal.json`, "utf8"),
) as {
  entries: Array<{ idx: number; tag: string }>;
};

describe("production release sequencing", () => {
  test("keeps Git deployments enabled so Vercel builds the exact main SHA", () => {
    expect(vercelConfig.git).toBeUndefined();
  });

  test("serializes the exact commit through verification and the reviewer-gated migration", () => {
    expect(workflow).toContain("branches: [main]");
    expect(workflow).toContain("group: filecheap-production-release");
    expect(workflow).toContain("cancel-in-progress: false");
    expect(workflow.match(/ref: \$\{\{ github\.sha \}\}/g)).toHaveLength(2);
    expect(workflow).toContain("needs: verify");
    expect(workflow.match(/^    environment: production$/gm)).toHaveLength(1);
    expect(workflow.match(/^    name: Production verification$/gm)).toHaveLength(
      1,
    );
    expect(workflow.match(/^    name: Production migration gate$/gm)).toHaveLength(
      1,
    );
    expect(workflow).toContain(
      "MIGRATIONS_DATABASE_URL: ${{ secrets.MIGRATIONS_DATABASE_URL }}",
    );
    expect(workflow).toContain(
      "MIGRATIONS_DATABASE_URL must use the direct, non-pooled Neon host.",
    );
    expect(workflow).not.toMatch(
      /^\s*DATABASE_URL: \$\{\{ secrets\.MIGRATIONS_DATABASE_URL \}\}$/m,
    );
  });

  test("uses pinned actions and Bun without deployment credentials or a second deployer", () => {
    expect(workflow).toContain(
      "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1",
    );
    expect(workflow).toContain(
      "actions/setup-go@4dc6199c7b1a012772edbd06daecab0f50c9053c",
    );
    expect(workflow).toContain("bun run db:migrate");
    expect(workflow).not.toContain("VERCEL_TOKEN");
    expect(workflow).not.toContain("VERCEL_ORG_ID");
    expect(workflow).not.toContain("VERCEL_PROJECT_ID");
    expect(workflow).not.toMatch(/\bvercel\s+(?:pull|build|deploy)\b/);
    expect(workflow).not.toMatch(/\b(?:npm|npx|yarn|pnpm)\b/);
  });

  test("keeps every committed SQL migration in the Drizzle journal", () => {
    const sqlTags = readdirSync(`${process.cwd()}/drizzle`)
      .filter((name) => name.endsWith(".sql"))
      .map((name) => name.slice(0, -4))
      .sort();
    const journalTags = [...migrationJournal.entries]
      .sort((left, right) => left.idx - right.idx)
      .map((entry) => entry.tag);

    expect(journalTags).toEqual(sqlTags);
  });
});
