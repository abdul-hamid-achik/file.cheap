import { neon } from "@neondatabase/serverless";

import {
  assertExactConsoleOwner,
  ConsoleOwnerCheckError,
  parseConsoleOwnerCheckInput,
} from "./check-console-owner-core";

async function checkConsoleOwner(): Promise<void> {
  const input = parseConsoleOwnerCheckInput(process.env);
  const sql = neon(input.databaseUrl);

  let rows: Record<string, unknown>[];
  try {
    rows = await sql`
      SELECT id, email
      FROM console_users
      WHERE id = ${input.ownerAccountId} OR lower(email) = ${input.ownerEmail}
      ORDER BY id
      LIMIT 3
    `;
  } catch {
    throw new ConsoleOwnerCheckError(
      "Console owner preflight could not read console_users.",
    );
  }

  assertExactConsoleOwner(
    input,
    rows.map((row) => ({ id: row.id, email: row.email })),
  );

  process.stdout.write(
    `Console owner preflight passed using ${input.databaseEnvironmentVariable}.\n`,
  );
}

try {
  await checkConsoleOwner();
} catch (error) {
  const message =
    error instanceof ConsoleOwnerCheckError
      ? error.message
      : "Console owner preflight failed unexpectedly.";
  process.stderr.write(`${message}\n`);
  process.exitCode = 1;
}
