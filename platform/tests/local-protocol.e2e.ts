import { createHash, randomBytes, randomUUID } from "node:crypto";
import { mkdtemp, rm } from "node:fs/promises";
import { createServer } from "node:net";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";

const platformRoot = resolve(import.meta.dir, "..");
const port = await reservePort();
const dataDirectory = await mkdtemp(
  join(tmpdir(), "filecheap-platform-e2e-"),
);
const baseUrl = `http://127.0.0.1:${port}`;
const apiToken = `e2e_${randomBytes(24).toString("hex")}`;
const signingSecret = randomBytes(48).toString("base64url");
const serverEnvironment: Record<string, string | undefined> = {
  ...process.env,
  BLOB_READ_WRITE_TOKEN: undefined,
  NEXT_TELEMETRY_DISABLED: "1",
  PLATFORM_API_TOKEN: apiToken,
  PLATFORM_DATA_DIR: dataDirectory,
  PLATFORM_PUBLIC_URL: baseUrl,
  PLATFORM_SIGNING_SECRET: signingSecret,
  PLATFORM_STORAGE_DRIVER: "local",
};
delete serverEnvironment.NODE_ENV;
delete serverEnvironment.VERCEL;
delete serverEnvironment.VERCEL_ENV;

const uniqueValue = `${Date.now()}-${randomUUID()}`;
const bytes = new TextEncoder().encode(
  `file.cheap isolated recovery e2e ${uniqueValue}\n`,
);
const sha256 = hash(bytes);
const stashId = `local-e2e-${uniqueValue}`;
const contentType = "application/vnd.filecheap.stash";

let server: RunningServer | undefined;

try {
  server = await startServer();
  const firstPid = server.process.pid;

  const healthRequestId = requestId("health-first");
  const health = await requestJson<HealthResponse>(
    "/api/v1/health",
    { headers: { "x-request-id": healthRequestId } },
    healthRequestId,
  );
  assert(health.status === "ok", "health endpoint did not report ok");
  assert(health.storage === "local", "health endpoint did not use local storage");
  assert(
    health.storageVerification === "server-sha256",
    "local storage did not advertise server SHA-256 verification",
  );
  assert(health.database === "none", "prototype unexpectedly required a database");

  const unauthorizedRequestId = requestId("auth-denied");
  const unauthorized = await fetch(`${baseUrl}/api/v1/stashes`, {
    headers: { "x-request-id": unauthorizedRequestId },
  });
  assertResponseMetadata(unauthorized, unauthorizedRequestId);
  assert(unauthorized.status === 401, "stashes endpoint accepted missing auth");
  assert(
    unauthorized.headers.get("content-type")?.includes("application/problem+json"),
    "auth failure was not returned as problem details",
  );
  const authProblem = (await unauthorized.json()) as ProblemDetails;
  assert(authProblem.code === "unauthorized", "auth failure used the wrong problem code");
  assert(
    authProblem.requestId === unauthorizedRequestId,
    "problem body did not preserve the request ID",
  );

  const planRequestId = requestId("plan");
  const plan = await api<PlanResponse>(
    "/api/v1/sync/plans",
    { contentType, sha256, sizeBytes: bytes.byteLength, stashId },
    planRequestId,
    201,
  );
  assert(plan.state === "upload_required", `unexpected plan state: ${plan.state}`);
  assert(plan.upload, "new object did not receive an upload grant");

  const uploadRequestId = requestId("upload");
  const upload = await fetch(plan.upload.url, {
    body: bytes,
    headers: {
      ...plan.upload.headers,
      "x-request-id": uploadRequestId,
    },
    method: plan.upload.method,
  });
  assertResponseMetadata(upload, uploadRequestId);
  if (upload.status !== 201) {
    throw new Error(`upload failed: ${upload.status} ${await upload.text()}`);
  }
  await upload.arrayBuffer();

  const commitRequestId = requestId("commit");
  const commit = await api<CommitResponse>(
    "/api/v1/sync/commits",
    { receipt: plan.receipt },
    commitRequestId,
  );
  assert(commit.requiresFullVerification, "commit did not require deep verification");
  assert(commit.stash.stashId === stashId, "commit returned a different stash ID");
  assert(commit.stash.sha256 === sha256, "commit returned a different SHA-256");
  assert(
    commit.stash.storageVerification === "server-sha256",
    "commit did not report the adapter verification level",
  );

  const stopped = await stopServer(server);
  server = undefined;
  assert(stopped.exitCode === 0, formatServerFailure("first server shutdown", stopped));

  server = await startServer();
  assert(server.process.pid !== firstPid, "restart reused the original server process");

  const restartHealthId = requestId("health-restart");
  const restartedHealth = await requestJson<HealthResponse>(
    "/api/v1/health",
    { headers: { "x-request-id": restartHealthId } },
    restartHealthId,
  );
  assert(restartedHealth.status === "ok", "restarted server did not become healthy");

  const listRequestId = requestId("list-after-restart");
  const listing = await get<{ stashes: CloudStash[] }>(
    "/api/v1/stashes",
    listRequestId,
  );
  const persisted = listing.stashes.find((stash) => stash.stashId === stashId);
  assert(persisted, "committed stash did not persist across the server restart");
  assert(persisted.sha256 === sha256, "persisted stash SHA-256 changed after restart");

  const repeatedRequestId = requestId("plan-repeated");
  const repeated = await api<PlanResponse>(
    "/api/v1/sync/plans",
    { contentType, sha256, sizeBytes: bytes.byteLength, stashId },
    repeatedRequestId,
    201,
  );
  assert(repeated.state === "already_committed", "idempotent plan was not recognized");
  assert(repeated.upload === null, "idempotent plan issued an unnecessary upload");

  const downloadRequestId = requestId("download-plan");
  const download = await api<DownloadResponse>(
    "/api/v1/sync/downloads",
    { stashId },
    downloadRequestId,
    201,
  );
  assert(download.mustVerifySha256, "download plan did not require SHA-256 verification");

  const objectDownloadRequestId = requestId("download-object");
  const recoveredResponse = await fetch(download.grant.url, {
    headers: {
      ...download.grant.headers,
      "x-request-id": objectDownloadRequestId,
    },
    method: download.grant.method,
  });
  assertResponseMetadata(recoveredResponse, objectDownloadRequestId);
  if (!recoveredResponse.ok) {
    throw new Error(
      `download failed: ${recoveredResponse.status} ${await recoveredResponse.text()}`,
    );
  }
  const recovered = new Uint8Array(await recoveredResponse.arrayBuffer());
  assert(recovered.byteLength === download.expected.sizeBytes, "recovered size differs");
  assert(hash(recovered) === download.expected.sha256, "recovered SHA-256 differs");
  assert(hash(recovered) === sha256, "recovered bytes differ from the original upload");

  console.log(
    JSON.stringify(
      {
        bytes: recovered.byteLength,
        dataDirectory,
        deepVerification: "passed",
        restarted: true,
        sha256,
        stashId,
        storage: restartedHealth.storage,
      },
      null,
      2,
    ),
  );
} finally {
  if (server) {
    await stopServer(server);
  }
  await rm(dataDirectory, { force: true, recursive: true });
}

type CloudStash = {
  sha256: string;
  stashId: string;
  storageVerification: string;
};

type CommitResponse = {
  requiresFullVerification: boolean;
  stash: CloudStash;
};

type DownloadResponse = {
  expected: { sha256: string; sizeBytes: number };
  grant: TransferGrant;
  mustVerifySha256: boolean;
};

type HealthResponse = {
  database: string;
  status: string;
  storage: string;
  storageVerification: string;
};

type PlanResponse = {
  receipt: string;
  state: "already_committed" | "object_present" | "upload_required";
  upload: TransferGrant | null;
};

type ProblemDetails = {
  code: string;
  requestId: string;
};

type RunningServer = {
  process: Bun.Subprocess<"ignore", "pipe", "pipe">;
  stderr: Promise<string>;
  stdout: Promise<string>;
};

type ServerExit = {
  exitCode: number;
  stderr: string;
  stdout: string;
};

type TransferGrant = {
  headers: Record<string, string>;
  method: "GET" | "PUT";
  url: string;
};

async function api<T>(
  path: string,
  body: unknown,
  requestId: string,
  expectedStatus = 200,
): Promise<T> {
  return requestJson<T>(
    path,
    {
      body: JSON.stringify(body),
      headers: {
        authorization: `Bearer ${apiToken}`,
        "content-type": "application/json",
        "x-request-id": requestId,
      },
      method: "POST",
    },
    requestId,
    expectedStatus,
  );
}

async function get<T>(path: string, requestId: string): Promise<T> {
  return requestJson<T>(
    path,
    {
      headers: {
        authorization: `Bearer ${apiToken}`,
        "x-request-id": requestId,
      },
    },
    requestId,
  );
}

async function requestJson<T>(
  path: string,
  init: RequestInit,
  requestId: string,
  expectedStatus = 200,
): Promise<T> {
  const response = await fetch(`${baseUrl}${path}`, init);
  assertResponseMetadata(response, requestId);
  if (response.status !== expectedStatus) {
    throw new Error(
      `${path} failed: expected ${expectedStatus}, got ${response.status} ${await response.text()}`,
    );
  }
  return response.json() as Promise<T>;
}

function assertResponseMetadata(response: Response, requestId: string): void {
  assert(
    response.headers.get("x-request-id") === requestId,
    `response did not preserve request ID ${requestId}`,
  );
  assert(
    response.headers.get("cache-control") === "no-store",
    "response did not disable shared caching",
  );
  assert(
    response.headers.get("x-content-type-options") === "nosniff",
    "response did not include nosniff protection",
  );
}

async function startServer(): Promise<RunningServer> {
  const nextCli = join(platformRoot, "node_modules", "next", "dist", "bin", "next");
  const process = Bun.spawn(
    [
      globalThis.process.execPath,
      nextCli,
      "dev",
      "--hostname",
      "127.0.0.1",
      "--port",
      String(port),
    ],
    {
      cwd: platformRoot,
      env: serverEnvironment,
      stderr: "pipe",
      stdin: "ignore",
      stdout: "pipe",
    },
  );
  const running: RunningServer = {
    process,
    stderr: new Response(process.stderr).text(),
    stdout: new Response(process.stdout).text(),
  };

  try {
    await waitForHealth(running);
    return running;
  } catch (error) {
    const stopped = await stopServer(running);
    throw new Error(
      `${error instanceof Error ? error.message : String(error)}\n${formatServerFailure("server startup", stopped)}`,
    );
  }
}

async function waitForHealth(running: RunningServer): Promise<void> {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (running.process.exitCode !== null) {
      throw new Error(`Next exited before health was ready (${running.process.exitCode})`);
    }
    try {
      const response = await fetch(`${baseUrl}/api/v1/health`, {
        headers: { "x-request-id": requestId("health-probe") },
        signal: AbortSignal.timeout(1_000),
      });
      if (response.ok) {
        await response.arrayBuffer();
        return;
      }
    } catch {
      // The dedicated server is still binding or compiling the health route.
    }
    await Bun.sleep(200);
  }
  throw new Error(`Next did not become healthy at ${baseUrl} within 30 seconds`);
}

async function stopServer(running: RunningServer): Promise<ServerExit> {
  if (running.process.exitCode === null) {
    running.process.kill("SIGTERM");
    const exitedGracefully = await Promise.race([
      running.process.exited.then(() => true),
      Bun.sleep(5_000).then(() => false),
    ]);
    if (!exitedGracefully && running.process.exitCode === null) {
      running.process.kill("SIGKILL");
    }
  }

  const [exitCode, stdout, stderr] = await Promise.all([
    running.process.exited,
    running.stdout,
    running.stderr,
  ]);
  return { exitCode, stderr, stdout };
}

async function reservePort(): Promise<number> {
  const listener = createServer();
  const port = await new Promise<number>((resolvePort, reject) => {
    listener.once("error", reject);
    listener.listen(0, "127.0.0.1", () => {
      const address = listener.address();
      if (!address || typeof address === "string") {
        reject(new Error("Could not reserve a dedicated TCP port"));
        return;
      }
      resolvePort(address.port);
    });
  });
  await new Promise<void>((resolveClose, reject) => {
    listener.close((error) => (error ? reject(error) : resolveClose()));
  });
  return port;
}

function formatServerFailure(stage: string, stopped: ServerExit): string {
  return [
    `${stage}: exit ${stopped.exitCode}`,
    stopped.stdout.trim() ? `stdout:\n${stopped.stdout.trim()}` : "",
    stopped.stderr.trim() ? `stderr:\n${stopped.stderr.trim()}` : "",
  ]
    .filter(Boolean)
    .join("\n");
}

function requestId(stage: string): string {
  return `e2e-${stage}-${randomUUID()}`;
}

function hash(value: Uint8Array): string {
  return createHash("sha256").update(value).digest("hex");
}

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}
