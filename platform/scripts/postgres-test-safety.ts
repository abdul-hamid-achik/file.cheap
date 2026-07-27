const loopbackHosts = new Set(["127.0.0.1", "localhost", "[::1]"]);

export type PostgresTestTarget = {
  databaseName: string;
  databaseUrl: string;
  host: string;
  remote: boolean;
};

export function parsePostgresTestTarget(
  value: string | undefined,
  options: { allowRemote: boolean },
): PostgresTestTarget {
  if (!value?.trim()) {
    throw new Error(
      "FILECHEAP_POSTGRES_TEST_URL is required when Docker is not managing the test database.",
    );
  }

  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error("FILECHEAP_POSTGRES_TEST_URL must be a valid URL.");
  }

  if (parsed.protocol !== "postgres:" && parsed.protocol !== "postgresql:") {
    throw new Error(
      "FILECHEAP_POSTGRES_TEST_URL must use the postgres or postgresql protocol.",
    );
  }

  const databaseName = decodeURIComponent(parsed.pathname.replace(/^\//, ""));
  if (!databaseName || !/(?:^|[-_])test(?:[-_]|$)/i.test(databaseName)) {
    throw new Error(
      "FILECHEAP_POSTGRES_TEST_URL must name an unmistakable test database (for example filecheap_test).",
    );
  }

  const remote = !loopbackHosts.has(parsed.hostname.toLowerCase());
  if (remote && !options.allowRemote) {
    throw new Error(
      "Remote test databases require FILECHEAP_ALLOW_REMOTE_TEST_DATABASE=true.",
    );
  }

  return {
    databaseName,
    databaseUrl: value,
    host: parsed.hostname,
    remote,
  };
}
