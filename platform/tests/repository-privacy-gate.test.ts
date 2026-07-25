import { afterEach, describe, expect, test } from "bun:test";
import { spawnSync } from "node:child_process";
import {
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const repositoryRoot = `${process.cwd()}/..`;
const privacyGate = `${repositoryRoot}/scripts/check-repository-privacy.sh`;
const temporaryRepositories: string[] = [];

function makeRepository(files: Record<string, string>): string {
  const directory = mkdtempSync(join(tmpdir(), "filecheap-privacy-gate-"));
  temporaryRepositories.push(directory);
  spawnSync("git", ["init", "--quiet"], { cwd: directory });

  for (const [name, content] of Object.entries(files)) {
    writeFileSync(join(directory, name), content);
  }

  const add = spawnSync("git", ["add", "."], {
    cwd: directory,
    encoding: "utf8",
  });
  expect(add.status).toBe(0);
  return directory;
}

function runGate(directory: string) {
  return spawnSync("bash", [privacyGate, directory], {
    cwd: repositoryRoot,
    encoding: "utf8",
  });
}

afterEach(() => {
  for (const directory of temporaryRepositories.splice(0)) {
    rmSync(directory, { recursive: true, force: true });
  }
});

describe("repository privacy gate", () => {
  test("accepts neutral examples and legitimate vidtrace references", () => {
    const repository = makeRepository({
      "README.md": "Investigate EXAMPLE-1234 with vidtrace.\n",
    });

    expect(runGate(repository).status).toBe(0);
  });

  test("rejects prohibited markers in tracked text without echoing content", () => {
    const formerTool = ["Graph", "ite"].join("");
    const formerTicket = `${String.fromCharCode(79, 80, 71)}-15061`;
    const formerPhrase = ["Internal", "Migrant"].join(" ");
    const formerSymbol = ["INTEL", "Workers", "ITA", "International"].join("_");
    const repository = makeRepository({
      "tool.txt": `old review tool: ${formerTool}\n`,
      "ticket.txt": `old ticket: ${formerTicket}\n`,
      "phrase.txt": `old vocabulary: ${formerPhrase}\n`,
      "symbol.txt": `old symbol: ${formerSymbol}\n`,
    });

    const result = runGate(repository);
    expect(result.status).toBe(1);
    expect(result.stderr).toContain("tool.txt");
    expect(result.stderr).toContain("ticket.txt");
    expect(result.stderr).toContain("phrase.txt");
    expect(result.stderr).toContain("symbol.txt");
    expect(result.stderr).not.toContain(formerTool);
    expect(result.stderr).not.toContain(formerTicket);
    expect(result.stderr).not.toContain(formerPhrase);
    expect(result.stderr).not.toContain(formerSymbol);
  });

  test("rejects prohibited markers in tracked paths", () => {
    const formerTicket = `${String.fromCharCode(79, 80, 71)}-15061`;
    const repository = makeRepository({
      [`${formerTicket}.txt`]: "fixture\n",
    });

    const result = runGate(repository);
    expect(result.status).toBe(1);
    expect(result.stderr).toContain(`${formerTicket}.txt`);
  });

  test("runs in CI and every production release verification", () => {
    const ciWorkflow = readFileSync(
      `${repositoryRoot}/.github/workflows/ci.yml`,
      "utf8",
    );
    const productionWorkflow = readFileSync(
      `${repositoryRoot}/.github/workflows/production-release.yml`,
      "utf8",
    );
    const gateCommand = "bash scripts/check-repository-privacy.sh";

    expect(ciWorkflow).toContain(gateCommand);
    expect(productionWorkflow).toContain(gateCommand);
  });
});
