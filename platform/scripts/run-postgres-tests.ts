import { randomUUID } from "node:crypto";
import { readdirSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { Pool } from "pg";

import { parsePostgresTestTarget } from "./postgres-test-safety";

const projectDirectory = fileURLToPath(new URL("..", import.meta.url)).replace(
  /\/$/,
  "",
);
const postgresImage =
  "postgres:16-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777";
const dockerUser = "filecheap_test";
const dockerPassword = "filecheap-postgres-test-only";
const dockerDatabase = "filecheap_test";
const requestedTests = process.argv.slice(2);
const postgresIntegrationTests = requestedTests.length > 0
  ? requestedTests
  : readdirSync(`${projectDirectory}/tests`)
      .filter((name) => name.endsWith(".postgres.integration.test.ts"))
      .sort()
      .map((name) => `tests/${name}`);

if (postgresIntegrationTests.length === 0) {
  throw new Error(
    "Expected at least one tests/*.postgres.integration.test.ts file.",
  );
}

let managedContainer: string | undefined;

try {
  const databaseUrl = process.env.FILECHEAP_POSTGRES_TEST_URL
    ? await prepareExplicitDatabase(process.env.FILECHEAP_POSTGRES_TEST_URL)
    : await startManagedPostgres();

  await waitForPostgres(databaseUrl);
  await assertDatabaseIsEmpty(databaseUrl);
  await run(["bun", "run", "db:migrate"], {
    MIGRATIONS_DATABASE_URL: databaseUrl,
  });
  // Drizzle records each applied hash. A second pass must be a no-op rather
  // than replaying schema changes or producing duplicate ledger entries.
  await run(["bun", "run", "db:migrate"], {
    MIGRATIONS_DATABASE_URL: databaseUrl,
  });
  await run(
    ["bun", "test", ...postgresIntegrationTests],
    { FILECHEAP_POSTGRES_TEST_URL: databaseUrl },
  );
} finally {
  if (managedContainer) {
    await run(["docker", "rm", "--force", managedContainer], {}, true);
    console.info("Removed the ephemeral PostgreSQL test container.");
  }
}

async function prepareExplicitDatabase(databaseUrl: string): Promise<string> {
  const target = parsePostgresTestTarget(databaseUrl, {
    allowRemote:
      process.env.FILECHEAP_ALLOW_REMOTE_TEST_DATABASE?.toLowerCase() === "true",
  });
  console.info(
    `Using the explicit ${target.remote ? "remote" : "loopback"} PostgreSQL test database ${target.databaseName}.`,
  );
  return target.databaseUrl;
}

async function startManagedPostgres(): Promise<string> {
  managedContainer = `filecheap-postgres-test-${process.pid}-${randomUUID().slice(0, 8)}`;
  console.info("Starting an ephemeral PostgreSQL 16 test container.");
  await run([
    "docker",
    "run",
    "--detach",
    "--rm",
    "--name",
    managedContainer,
    "--env",
    `POSTGRES_USER=${dockerUser}`,
    "--env",
    `POSTGRES_PASSWORD=${dockerPassword}`,
    "--env",
    `POSTGRES_DB=${dockerDatabase}`,
    "--publish",
    "127.0.0.1::5432",
    "--tmpfs",
    "/var/lib/postgresql/data:rw,noexec,nosuid,size=256m",
    postgresImage,
  ]);

  for (let attempt = 0; attempt < 80; attempt += 1) {
    const ready = await run(
      [
        "docker",
        "exec",
        managedContainer,
        "pg_isready",
        "--username",
        dockerUser,
        "--dbname",
        dockerDatabase,
      ],
      {},
      true,
      true,
    );
    if (ready) break;
    if (attempt === 79) {
      throw new Error("Ephemeral PostgreSQL did not become ready in time.");
    }
    await Bun.sleep(250);
  }

  const portOutput = await capture([
    "docker",
    "port",
    managedContainer,
    "5432/tcp",
  ]);
  const port = portOutput.match(/:(\d+)\s*$/m)?.[1];
  if (!port) {
    throw new Error("Docker did not report the PostgreSQL test port.");
  }
  return `postgresql://${dockerUser}:${dockerPassword}@127.0.0.1:${port}/${dockerDatabase}?sslmode=disable`;
}

async function assertDatabaseIsEmpty(databaseUrl: string): Promise<void> {
  const pool = new Pool({ connectionString: databaseUrl, max: 1 });
  try {
    const result = await pool.query<{ table_name: string; table_schema: string }>(`
      SELECT table_schema, table_name
      FROM information_schema.tables
      WHERE table_type = 'BASE TABLE'
        AND table_schema IN ('public', 'drizzle')
      ORDER BY table_schema, table_name
    `);
    const tables = result.rows;
    if (tables.length > 0) {
      const names = tables
        .map((table) => `${table.table_schema}.${table.table_name}`)
        .join(", ");
      throw new Error(
        `PostgreSQL integration tests require an empty database; found ${names}.`,
      );
    }
  } finally {
    await pool.end();
  }
}

async function waitForPostgres(databaseUrl: string): Promise<void> {
  let lastError: unknown;
  for (let attempt = 0; attempt < 40; attempt += 1) {
    const pool = new Pool({
      connectionString: databaseUrl,
      connectionTimeoutMillis: 1_000,
      max: 1,
    });
    try {
      await pool.query("SELECT 1");
      await pool.end();
      return;
    } catch (error) {
      lastError = error;
      await pool.end().catch(() => undefined);
      if (attempt < 39) await Bun.sleep(250);
    }
  }
  throw new Error("PostgreSQL did not accept a stable test query in time.", {
    cause: lastError,
  });
}

async function capture(command: string[]): Promise<string> {
  const child = Bun.spawn(command, {
    cwd: projectDirectory,
    env: process.env,
    stderr: "pipe",
    stdout: "pipe",
  });
  const [exitCode, stdout, stderr] = await Promise.all([
    child.exited,
    new Response(child.stdout).text(),
    new Response(child.stderr).text(),
  ]);
  if (exitCode !== 0) {
    throw new Error(
      `${command[0]} failed with exit code ${exitCode}: ${stderr.trim()}`,
    );
  }
  return stdout.trim();
}

async function run(
  command: string[],
  extraEnvironment: Record<string, string> = {},
  tolerateFailure = false,
  quiet = false,
): Promise<boolean> {
  const child = Bun.spawn(command, {
    cwd: projectDirectory,
    env: { ...process.env, ...extraEnvironment },
    stderr: quiet ? "ignore" : "inherit",
    stdout: quiet ? "ignore" : "inherit",
  });
  const exitCode = await child.exited;
  if (exitCode !== 0 && !tolerateFailure) {
    throw new Error(
      `${command[0]} failed with exit code ${exitCode}.`,
    );
  }
  return exitCode === 0;
}
