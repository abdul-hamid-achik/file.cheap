import { createHash, randomBytes, randomUUID } from "node:crypto";
import { mkdtemp, rm } from "node:fs/promises";
import { createServer } from "node:net";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";

const platformRoot = resolve(import.meta.dir, "..");
const dataDirectory = await mkdtemp(
  join(tmpdir(), "filecheap-platform-e2e-"),
);
let port = 0;
let baseUrl = "";
const apiToken = `e2e_${randomBytes(24).toString("hex")}`;
const signingSecret = randomBytes(48).toString("base64url");
const serverEnvironment: Record<string, string | undefined> = {
  ...process.env,
  BLOB_READ_WRITE_TOKEN: undefined,
  NEXT_TELEMETRY_DISABLED: "1",
  NODE_ENV: "production",
  PLATFORM_API_TOKEN: apiToken,
  PLATFORM_DATA_DIR: dataDirectory,
  PLATFORM_PUBLIC_URL: "http://127.0.0.1:3100",
  PLATFORM_RECOVERY_LAB_ENABLED: "true",
  PLATFORM_SIGNING_SECRET: signingSecret,
  PLATFORM_STORAGE_DRIVER: "local",
};
delete serverEnvironment.VERCEL;
delete serverEnvironment.VERCEL_ENV;

const uniqueValue = `${Date.now()}-${randomUUID()}`;
const bytes = new TextEncoder().encode(
  `file.cheap isolated recovery e2e ${uniqueValue}\n`,
);
const sha256 = hash(bytes);
const stashId = `local-e2e-${uniqueValue}`;
const contentType = "application/vnd.filecheap.stash";
const retryBytes = new TextEncoder().encode(
  `file.cheap retry-safe upload ${uniqueValue}\n`,
);
const retrySha256 = hash(retryBytes);
const retryStashId = `retry-e2e-${uniqueValue}`;

let server: RunningServer | undefined;

try {
  await buildApplication();
  port = await reservePort();
  baseUrl = `http://127.0.0.1:${port}`;
  serverEnvironment.PLATFORM_PUBLIC_URL = baseUrl;
  server = await startServer();
  const firstPid = server.process.pid;

  const homepage = await timedFetch(baseUrl);
  assert(homepage.status === 200, "homepage did not load");
  const contentSecurityPolicy = homepage.headers.get("content-security-policy") ?? "";
  assert(
    contentSecurityPolicy.includes("frame-ancestors 'none'"),
    "homepage did not prevent framing",
  );
  assert(
    contentSecurityPolicy.includes("https://vercel.com") &&
      contentSecurityPolicy.includes("https://*.private.blob.vercel-storage.com"),
    "homepage CSP did not allow signed private Blob transfers",
  );
  assert(
    !contentSecurityPolicy.includes("'unsafe-eval'"),
    "production CSP unexpectedly allowed eval",
  );
  assert(
    homepage.headers.get("x-frame-options") === "DENY",
    "homepage did not send legacy frame protection",
  );
  const homepageHtml = await homepage.text();
  assert(
    homepageHtml.includes("Keep the files your agents create"),
    "homepage did not render the static local product",
  );
  assert(
    !homepageHtml.includes("Development bearer token"),
    "homepage exposed the experimental recovery client",
  );

  const lab = await timedFetch(`${baseUrl}/lab`);
  assert(lab.status === 200, "explicitly enabled recovery lab did not load");
  const labHtml = await lab.text();
  assert(
    labHtml.includes("Development bearer token"),
    "recovery lab did not render its controlled client",
  );
  assert(
    /<meta[^>]+name="robots"[^>]+content="noindex, nofollow, noarchive, nosnippet"/u.test(
      labHtml,
    ),
    "recovery lab did not emit a complete noindex policy",
  );

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
  const unauthorized = await timedFetch(`${baseUrl}/api/v1/stashes`, {
    headers: { "x-request-id": unauthorizedRequestId },
  });
  assertResponseMetadata(unauthorized, unauthorizedRequestId);
  assert(unauthorized.status === 401, "stashes endpoint accepted missing auth");
  assert(
    unauthorized.headers.get("content-type")?.includes("application/problem+json"),
    "auth failure was not returned as problem details",
  );
  assert(
    unauthorized.headers.get("www-authenticate") === 'Bearer realm="filecheap-platform"',
    "auth failure did not advertise the bearer challenge",
  );
  const authProblem = (await unauthorized.json()) as ProblemDetails;
  assert(authProblem.code === "unauthorized", "auth failure used the wrong problem code");
  assert(
    authProblem.requestId === unauthorizedRequestId,
    "problem body did not preserve the request ID",
  );

  const wrongAuthRequestId = requestId("auth-wrong");
  const wrongAuth = await timedFetch(`${baseUrl}/api/v1/stashes`, {
    headers: {
      authorization: "Bearer definitely-not-the-token",
      "x-request-id": wrongAuthRequestId,
    },
  });
  assertResponseMetadata(wrongAuth, wrongAuthRequestId);
  assert(wrongAuth.status === 401, "stashes endpoint accepted the wrong token");
  await wrongAuth.arrayBuffer();

  const wrongMethodRequestId = requestId("wrong-method");
  const wrongMethod = await timedFetch(`${baseUrl}/api/v1/sync/plans`, {
    headers: { "x-request-id": wrongMethodRequestId },
  });
  assertResponseMetadata(wrongMethod, wrongMethodRequestId);
  assert(wrongMethod.status === 405, "unsupported method did not return 405");
  assert(wrongMethod.headers.get("allow") === "POST", "405 omitted Allow: POST");
  assert(
    ((await wrongMethod.json()) as ProblemDetails).code === "method_not_allowed",
    "unsupported method used the wrong problem code",
  );

  const missingRouteRequestId = requestId("missing-api-route");
  const missingRoute = await timedFetch(`${baseUrl}/api/v1/definitely-missing`, {
    headers: { "x-request-id": missingRouteRequestId },
  });
  assertResponseMetadata(missingRoute, missingRouteRequestId);
  assert(missingRoute.status === 404, "unknown API route did not return 404");
  assert(
    ((await missingRoute.json()) as ProblemDetails).code === "api_route_not_found",
    "unknown API route did not use problem details",
  );

  const mixedCaseAuthRequestId = requestId("auth-case-insensitive");
  const mixedCaseAuth = await timedFetch(`${baseUrl}/api/v1/stashes`, {
    headers: {
      authorization: `bEaReR ${apiToken}`,
      "x-request-id": mixedCaseAuthRequestId,
    },
  });
  assertResponseMetadata(mixedCaseAuth, mixedCaseAuthRequestId);
  assert(mixedCaseAuth.status === 200, "bearer scheme was not case-insensitive");
  await mixedCaseAuth.arrayBuffer();

  const malformedRequestId = requestId("malformed-json");
  const malformed = await timedFetch(`${baseUrl}/api/v1/sync/plans`, {
    body: "{",
    headers: {
      authorization: `Bearer ${apiToken}`,
      "content-type": "application/json",
      "x-request-id": malformedRequestId,
    },
    method: "POST",
  });
  assertResponseMetadata(malformed, malformedRequestId);
  assert(malformed.status === 400, "malformed JSON did not return 400");
  assert(
    ((await malformed.json()) as ProblemDetails).code === "invalid_json",
    "malformed JSON used the wrong problem code",
  );

  const unsupportedMediaTypeRequestId = requestId("unsupported-media-type");
  const unsupportedMediaType = await timedFetch(`${baseUrl}/api/v1/sync/plans`, {
    body: JSON.stringify({ contentType, sha256, sizeBytes: bytes.byteLength, stashId }),
    headers: {
      authorization: `Bearer ${apiToken}`,
      "content-type": "text/plain",
      "x-request-id": unsupportedMediaTypeRequestId,
    },
    method: "POST",
  });
  assertResponseMetadata(unsupportedMediaType, unsupportedMediaTypeRequestId);
  assert(
    unsupportedMediaType.status === 415,
    "non-JSON request media type did not return 415",
  );
  assert(
    ((await unsupportedMediaType.json()) as ProblemDetails).code ===
      "unsupported_media_type",
    "unsupported media type used the wrong problem code",
  );

  const oversizedJsonRequestId = requestId("json-too-large");
  const oversizedJson = await timedFetch(`${baseUrl}/api/v1/sync/plans`, {
    body: JSON.stringify({ padding: "x".repeat(16 * 1024) }),
    headers: {
      authorization: `Bearer ${apiToken}`,
      "content-type": "application/json",
      "x-request-id": oversizedJsonRequestId,
    },
    method: "POST",
  });
  assertResponseMetadata(oversizedJson, oversizedJsonRequestId);
  assert(oversizedJson.status === 413, "oversized JSON did not return 413");
  assert(
    ((await oversizedJson.json()) as ProblemDetails).code === "payload_too_large",
    "oversized JSON used the wrong problem code",
  );

  const strictRequestId = requestId("strict-contract");
  const strictProblem = await api<ProblemDetails>(
    "/api/v1/sync/plans",
    {
      contentType,
      sha256,
      sizeBytes: bytes.byteLength,
      stashId,
      unexpected: "field",
    },
    strictRequestId,
    422,
  );
  assert(strictProblem.code === "invalid_request", "unknown plan fields were accepted");

  const missingDownloadRequestId = requestId("missing-download");
  const missingDownload = await api<ProblemDetails>(
    "/api/v1/sync/downloads",
    { stashId: `missing-${uniqueValue}` },
    missingDownloadRequestId,
    404,
  );
  assert(missingDownload.code === "stash_not_found", "missing stash used wrong problem");

  const incompleteBytes = new TextEncoder().encode(`not-uploaded ${uniqueValue}\n`);
  const incompleteRequestId = requestId("plan-incomplete");
  const incompletePlan = await api<PlanResponse>(
    "/api/v1/sync/plans",
    {
      contentType,
      sha256: hash(incompleteBytes),
      sizeBytes: incompleteBytes.byteLength,
      stashId: `incomplete-${uniqueValue}`,
    },
    incompleteRequestId,
    201,
  );
  const prematureCommitRequestId = requestId("commit-before-upload");
  const prematureCommit = await api<ProblemDetails>(
    "/api/v1/sync/commits",
    { receipt: incompletePlan.receipt },
    prematureCommitRequestId,
    409,
  );
  assert(
    prematureCommit.code === "upload_incomplete",
    "commit-before-upload used the wrong problem code",
  );

  const retryPlanRequestId = requestId("plan-retry-upload");
  const retryPlan = await api<PlanResponse>(
    "/api/v1/sync/plans",
    {
      contentType,
      sha256: retrySha256,
      sizeBytes: retryBytes.byteLength,
      stashId: retryStashId,
    },
    retryPlanRequestId,
    201,
  );
  assert(retryPlan.upload, "retry-safety plan did not issue an upload grant");
  assert(
    !new URL(retryPlan.upload.url).searchParams.has("token"),
    "local upload grant leaked its capability in the URL",
  );
  assert(
    Boolean(retryPlan.upload.headers["x-filecheap-transfer-token"]),
    "local upload grant omitted its capability header",
  );

  const missingTransferHeaderRequestId = requestId("missing-transfer-header");
  const missingTransferHeader = await timedFetch(retryPlan.upload.url, {
    body: retryBytes,
    headers: {
      "content-type": contentType,
      "x-request-id": missingTransferHeaderRequestId,
    },
    method: "PUT",
  });
  assertResponseMetadata(missingTransferHeader, missingTransferHeaderRequestId);
  assert(
    missingTransferHeader.status === 400,
    "local upload accepted a capability-free request",
  );
  assert(
    ((await missingTransferHeader.json()) as ProblemDetails).code === "missing_grant",
    "capability-free upload used the wrong problem code",
  );

  const oversizedRequestId = requestId("upload-too-large");
  const oversizedUpload = await uploadGrant(
    retryPlan.upload,
    concatBytes(retryBytes, new Uint8Array([0])),
    oversizedRequestId,
  );
  assert(oversizedUpload.status === 413, "oversized upload did not return 413");
  assert(
    ((await oversizedUpload.json()) as ProblemDetails).code === "upload_too_large",
    "oversized upload used the wrong problem code",
  );

  const wrongHashBytes = retryBytes.slice();
  wrongHashBytes[0] = wrongHashBytes[0] === 120 ? 121 : 120;
  const wrongHashRequestId = requestId("upload-wrong-hash");
  const wrongHashUpload = await uploadGrant(
    retryPlan.upload,
    wrongHashBytes,
    wrongHashRequestId,
  );
  assert(wrongHashUpload.status === 422, "wrong-hash upload did not return 422");
  assert(
    ((await wrongHashUpload.json()) as ProblemDetails).code === "integrity_mismatch",
    "wrong-hash upload used the wrong problem code",
  );

  const recoveredUploadRequestId = requestId("upload-after-failure");
  const recoveredUpload = await uploadGrant(
    retryPlan.upload,
    retryBytes,
    recoveredUploadRequestId,
  );
  assert(recoveredUpload.status === 201, "valid retry after failed uploads was rejected");
  await recoveredUpload.arrayBuffer();

  const retryCommitRequestId = requestId("commit-retry-upload");
  const retryCommit = await api<CommitResponse>(
    "/api/v1/sync/commits",
    { receipt: retryPlan.receipt },
    retryCommitRequestId,
  );
  assert(retryCommit.stash.stashId === retryStashId, "retry upload committed wrong stash");

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
  const upload = await timedFetch(plan.upload.url, {
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

  const repeatedCommitRequestId = requestId("commit-repeated");
  const repeatedCommit = await api<CommitResponse>(
    "/api/v1/sync/commits",
    { receipt: plan.receipt },
    repeatedCommitRequestId,
  );
  assert(
    repeatedCommit.stash.stashId === commit.stash.stashId &&
      repeatedCommit.stash.sha256 === commit.stash.sha256,
    "repeated commit did not return the original logical result",
  );

  const stopped = await stopServer(server);
  server = undefined;
  assert(
    stopped.exitCode === 0 || stopped.exitCode === 143,
    formatServerFailure("first server shutdown", stopped),
  );

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
  assert(
    !new URL(download.grant.url).searchParams.has("token"),
    "local download grant leaked its capability in the URL",
  );
  assert(
    Boolean(download.grant.headers["x-filecheap-transfer-token"]),
    "local download grant omitted its capability header",
  );
  assert(download.mustVerifySha256, "download plan did not require SHA-256 verification");

  const objectDownloadRequestId = requestId("download-object");
  const recoveredResponse = await timedFetch(download.grant.url, {
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
  assert(
    /^"[^"]+"$/.test(recoveredResponse.headers.get("etag") ?? ""),
    "download ETag was not a quoted entity tag",
  );
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
  const response = await timedFetch(`${baseUrl}${path}`, init);
  assertResponseMetadata(response, requestId);
  if (response.status !== expectedStatus) {
    throw new Error(
      `${path} failed: expected ${expectedStatus}, got ${response.status} ${await response.text()}`,
    );
  }
  return response.json() as Promise<T>;
}

async function uploadGrant(
  grant: TransferGrant,
  body: Uint8Array,
  requestId: string,
): Promise<Response> {
  const requestBody = body.buffer instanceof ArrayBuffer
    ? body.buffer.slice(body.byteOffset, body.byteOffset + body.byteLength)
    : Uint8Array.from(body).buffer;
  const response = await timedFetch(grant.url, {
    body: requestBody,
    headers: {
      ...grant.headers,
      "x-request-id": requestId,
    },
    method: grant.method,
  });
  assertResponseMetadata(response, requestId);
  return response;
}

async function timedFetch(
  input: string | URL | Request,
  init: RequestInit = {},
): Promise<Response> {
  return fetch(input, {
    ...init,
    signal: init.signal ?? AbortSignal.timeout(10_000),
  });
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
      "start",
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

async function buildApplication(): Promise<void> {
  const nextCli = join(platformRoot, "node_modules", "next", "dist", "bin", "next");
  const build = Bun.spawn(
    [globalThis.process.execPath, nextCli, "build"],
    {
      cwd: platformRoot,
      env: serverEnvironment,
      stderr: "pipe",
      stdin: "ignore",
      stdout: "pipe",
    },
  );
  const stdoutPromise = new Response(build.stdout).text();
  const stderrPromise = new Response(build.stderr).text();
  let buildTimeout: ReturnType<typeof setTimeout> | undefined;
  const timeoutPromise = new Promise<true>((resolveTimeout) => {
    buildTimeout = setTimeout(() => resolveTimeout(true), 180_000);
  });
  const timedOut = await Promise.race([
    build.exited.then(() => false),
    timeoutPromise,
  ]);
  if (buildTimeout) clearTimeout(buildTimeout);
  if (timedOut && build.exitCode === null) {
    build.kill("SIGKILL");
  }
  const [exitCode, stdout, stderr] = await Promise.all([
    build.exited,
    stdoutPromise,
    stderrPromise,
  ]);
  if (timedOut) {
    throw new Error(
      `production build exceeded 180 seconds\n${stdout.trim()}\n${stderr.trim()}`,
    );
  }
  if (exitCode !== 0) {
    throw new Error(
      [
        `production build failed with exit ${exitCode}`,
        stdout.trim() ? `stdout:\n${stdout.trim()}` : "",
        stderr.trim() ? `stderr:\n${stderr.trim()}` : "",
      ]
        .filter(Boolean)
        .join("\n"),
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

function concatBytes(first: Uint8Array, second: Uint8Array): Uint8Array {
  const combined = new Uint8Array(first.byteLength + second.byteLength);
  combined.set(first);
  combined.set(second, first.byteLength);
  return combined;
}

function assert(condition: unknown, message: string): asserts condition {
  if (!condition) {
    throw new Error(message);
  }
}
