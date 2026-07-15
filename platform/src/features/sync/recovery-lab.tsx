"use client";

import { useMemo, useState } from "react";

import type { CloudStash } from "@/features/catalog/catalog";

type LabStatus = {
  kind: "error" | "idle" | "success" | "working";
  message: string;
};

type PlanResponse = {
  receipt: string;
  state: "already_committed" | "object_present" | "upload_required";
  upload: {
    headers: Record<string, string>;
    method: "PUT";
    url: string;
  } | null;
};

type DownloadResponse = {
  expected: { sha256: string; sizeBytes: number };
  grant: { headers: Record<string, string>; method: "GET"; url: string };
};

export function RecoveryLab({ initialStashes }: { initialStashes: CloudStash[] }) {
  const [apiToken, setApiToken] = useState("local-development-token");
  const [file, setFile] = useState<File | null>(null);
  const [stashId, setStashId] = useState("");
  const [stashes, setStashes] = useState(initialStashes);
  const [status, setStatus] = useState<LabStatus>({
    kind: "idle",
    message: "Choose a small archive to exercise the local protocol.",
  });

  const canPush = Boolean(file && stashId.trim() && status.kind !== "working");
  const totalBytes = useMemo(
    () => stashes.reduce((total, stash) => total + stash.sizeBytes, 0),
    [stashes],
  );

  async function pushArchive() {
    if (!file || !stashId.trim()) {
      return;
    }

    try {
      setStatus({ kind: "working", message: "Hashing the exact archive bytes…" });
      const bytes = await file.arrayBuffer();
      const sha256 = await sha256Hex(bytes);
      const contentType = "application/vnd.filecheap.stash";

      setStatus({ kind: "working", message: "Requesting an immutable upload plan…" });
      const plan = await apiRequest<PlanResponse>(
        "/api/v1/sync/plans",
        apiToken,
        {
          contentType,
          sha256,
          sizeBytes: file.size,
          stashId: stashId.trim(),
        },
      );

      if (plan.upload) {
        setStatus({ kind: "working", message: "Transferring through the signed data path…" });
        const uploadResponse = await fetch(plan.upload.url, {
          body: file,
          headers: plan.upload.headers,
          method: plan.upload.method,
        });
        if (!uploadResponse.ok) {
          throw await responseError(uploadResponse);
        }
      }

      setStatus({ kind: "working", message: "Committing the catalog receipt…" });
      await apiRequest("/api/v1/sync/commits", apiToken, { receipt: plan.receipt });
      const nextStashes = await listStashes(apiToken);
      setStashes(nextStashes);
      setStatus({
        kind: "success",
        message: `${stashId.trim()} committed. Download and verify it before trusting recovery.`,
      });
    } catch (error) {
      setStatus({ kind: "error", message: messageFor(error) });
    }
  }

  async function verifyArchive(stash: CloudStash) {
    try {
      setStatus({ kind: "working", message: `Downloading every byte of ${stash.stashId}…` });
      const plan = await apiRequest<DownloadResponse>(
        "/api/v1/sync/downloads",
        apiToken,
        { stashId: stash.stashId },
      );
      const response = await fetch(plan.grant.url, {
        headers: plan.grant.headers,
        method: plan.grant.method,
      });
      if (!response.ok) {
        throw await responseError(response);
      }
      const bytes = await response.arrayBuffer();
      const sha256 = await sha256Hex(bytes);
      if (bytes.byteLength !== plan.expected.sizeBytes || sha256 !== plan.expected.sha256) {
        throw new Error("Recovered bytes failed the expected size or SHA-256 check.");
      }
      setStatus({
        kind: "success",
        message: `${stash.stashId} recovered and verified byte for byte.`,
      });
    } catch (error) {
      setStatus({ kind: "error", message: messageFor(error) });
    }
  }

  return (
    <div className="labPanel">
      <div className="labToolbar">
        <label>
          <span>Development bearer token</span>
          <input
            autoComplete="off"
            onChange={(event) => setApiToken(event.target.value)}
            spellCheck={false}
            type="password"
            value={apiToken}
          />
        </label>
      </div>

      <div className="uploadRow">
        <label className="filePicker">
          <span>Archive</span>
          <input
            onChange={(event) => {
              const nextFile = event.target.files?.[0] ?? null;
              setFile(nextFile);
              if (nextFile && !stashId) {
                setStashId(nextFile.name.replace(/[^A-Za-z0-9._-]/g, "-").slice(0, 128));
              }
            }}
            type="file"
          />
          <strong>{file ? `${file.name} · ${formatBytes(file.size)}` : "Choose a test archive"}</strong>
        </label>
        <label className="stashField">
          <span>Stash ID</span>
          <input
            maxLength={128}
            onChange={(event) => setStashId(event.target.value)}
            placeholder="investigation-01"
            spellCheck={false}
            value={stashId}
          />
        </label>
        <button disabled={!canPush} onClick={pushArchive} type="button">
          Push archive
        </button>
      </div>

      <div className={`labStatus ${status.kind}`} role="status" aria-live="polite">
        <span aria-hidden="true" />
        {status.message}
      </div>

      <div className="vaultHeader">
        <div>
          <span>Committed vault</span>
          <strong>{stashes.length} objects · {formatBytes(totalBytes)}</strong>
        </div>
        <span className="tableHint">full GET + SHA-256</span>
      </div>

      {stashes.length === 0 ? (
        <div className="emptyVault">
          <span aria-hidden="true">□</span>
          <p>No committed remote stashes yet.</p>
        </div>
      ) : (
        <div className="stashList">
          {stashes.map((stash) => (
            <article key={stash.stashId}>
              <div>
                <strong>{stash.stashId}</strong>
                <span>{formatBytes(stash.sizeBytes)} · {shortHash(stash.sha256)}</span>
              </div>
              <button
                disabled={status.kind === "working"}
                onClick={() => verifyArchive(stash)}
                type="button"
              >
                Hydrate + verify
              </button>
            </article>
          ))}
        </div>
      )}
    </div>
  );
}

async function apiRequest<T>(path: string, token: string, body: unknown): Promise<T> {
  const response = await fetch(path, {
    body: JSON.stringify(body),
    headers: {
      authorization: `Bearer ${token}`,
      "content-type": "application/json",
    },
    method: "POST",
  });
  if (!response.ok) {
    throw await responseError(response);
  }
  return response.json() as Promise<T>;
}

async function listStashes(token: string): Promise<CloudStash[]> {
  const response = await fetch("/api/v1/stashes", {
    headers: { authorization: `Bearer ${token}` },
  });
  if (!response.ok) {
    throw await responseError(response);
  }
  const result = (await response.json()) as { stashes: CloudStash[] };
  return result.stashes;
}

async function responseError(response: Response): Promise<Error> {
  try {
    const problem = (await response.json()) as { detail?: string; title?: string };
    return new Error(problem.detail ?? problem.title ?? `Request failed (${response.status})`);
  } catch {
    return new Error(`Request failed (${response.status})`);
  }
}

async function sha256Hex(bytes: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, "0")).join("");
}

function messageFor(error: unknown): string {
  return error instanceof Error ? error.message : "The recovery lab failed unexpectedly.";
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 ** 2) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 ** 3) return `${(bytes / 1024 ** 2).toFixed(1)} MB`;
  return `${(bytes / 1024 ** 3).toFixed(1)} GB`;
}

function shortHash(hash: string): string {
  return `sha256:${hash.slice(0, 8)}…${hash.slice(-6)}`;
}
