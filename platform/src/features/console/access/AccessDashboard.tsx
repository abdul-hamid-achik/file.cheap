"use client";

import { useEffect, useRef, useState, type Ref } from "react";

import {
  accessDeviceListResponseSchema,
  accessDeviceRevokeResponseSchema,
  type AccessDevice,
  type AccessDeviceListResponse,
  type AccessDeviceOverview,
} from "@/features/auth/contracts";

import styles from "./access.module.css";

type AccessDevicePage = Omit<AccessDeviceListResponse, "version">;

interface AccessDashboardProps {
  initialPage: AccessDevicePage;
  now?: Date;
}

export function AccessDashboard({ initialPage, now = new Date() }: AccessDashboardProps) {
  const [accessPage, setAccessPage] = useState<AccessDevicePage>(initialPage);
  const [confirmingId, setConfirmingId] = useState<string | null>(null);
  const [revokingId, setRevokingId] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadError, setLoadError] = useState("");
  const [message, setMessage] = useState("");
  const confirmButtonRef = useRef<HTMLButtonElement>(null);
  const restoreRevokeFocusRef = useRef<string | null>(null);
  const revokeTriggerRefs = useRef(new Map<string, HTMLButtonElement>());
  const { devices, overview, pageInfo } = accessPage;
  const canLoadMore = pageInfo.hasNextPage && pageInfo.endCursor !== null;

  useEffect(() => {
    if (confirmingId) {
      confirmButtonRef.current?.focus();
      return;
    }
    const deviceId = restoreRevokeFocusRef.current;
    if (!deviceId) return;
    restoreRevokeFocusRef.current = null;
    revokeTriggerRefs.current.get(deviceId)?.focus();
  }, [confirmingId]);

  function cancelRevocation(deviceId: string) {
    restoreRevokeFocusRef.current = deviceId;
    setConfirmingId(null);
  }

  async function revoke(device: AccessDevice) {
    if (loadingMore || revokingId !== null) return;
    setMessage("");
    setRevokingId(device.id);
    try {
      const response = await fetch(
        `/api/console/access/devices/${encodeURIComponent(device.id)}`,
        { method: "DELETE" },
      );
      const payload = await response.json().catch(() => null) as unknown;
      if (!response.ok) {
        throw new Error(problemDetail(payload, "The device could not be revoked."));
      }
      const parsed = accessDeviceRevokeResponseSchema.safeParse(payload);
      if (!parsed.success || parsed.data.id !== device.id) {
        throw new Error("The access service returned an invalid revocation response.");
      }
      const revokedAt = new Date().toISOString();
      setAccessPage((current) => ({
        ...current,
        devices: current.devices.map((item) => item.id === device.id
          ? { ...item, revokedAt, status: "revoked" }
          : item),
        overview: accessOverviewAfterRevocation(current.overview, device, now),
      }));
      setConfirmingId(null);
      setMessage(`${device.clientName} can no longer refresh or use its device session.`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "The device could not be revoked.");
    } finally {
      setRevokingId(null);
    }
  }

  async function loadMore() {
    const cursor = pageInfo.endCursor;
    if (!pageInfo.hasNextPage || !cursor || loadingMore || revokingId !== null) return;
    setLoadError("");
    setLoadingMore(true);
    try {
      const response = await fetch(accessDeviceListHref(cursor, pageInfo.limit), {
        headers: { accept: "application/json" },
      });
      const payload = await response.json().catch(() => null) as unknown;
      if (!response.ok) {
        throw new Error(problemDetail(payload, "More devices could not be loaded."));
      }
      const parsed = accessDeviceListResponseSchema.safeParse(payload);
      if (!parsed.success) {
        throw new Error("The access service returned an invalid device page.");
      }
      setAccessPage((current) => appendAccessDevicePage(current, parsed.data));
    } catch (error) {
      setLoadError(error instanceof Error ? error.message : "More devices could not be loaded.");
    } finally {
      setLoadingMore(false);
    }
  }

  return (
    <section aria-busy={loadingMore || revokingId !== null} aria-labelledby="access-title" className={styles.page}>
      <header className={styles.header}>
        <div>
          <p className={styles.eyebrow}>Owner access control</p>
          <h1 id="access-title">Access</h1>
          <p>Review devices paired through the email approval flow and revoke durable access when a machine is retired or lost.</p>
        </div>
        <p className={styles.boundary}>Browser sessions are short-lived and end separately when you sign out.</p>
      </header>

      <dl aria-label="Device access summary" className={styles.metrics}>
        <Metric label="Total paired" value={overview.total} />
        <Metric label="Active devices" value={overview.active} />
        <Metric label="Expiring in 7 days" tone={overview.expiring > 0 ? "attention" : "quiet"} value={overview.expiring} />
        <Metric label="Revoked or expired" tone="quiet" value={overview.inactive} />
      </dl>

      <div className={styles.sectionHeader}>
        <div>
          <h2>Paired devices</h2>
          <p>Last refreshed is updated when a device rotates its credential; it is not a complete activity log.</p>
        </div>
        <p><strong>{devices.length}</strong> of {overview.total} loaded</p>
      </div>

      {overview.total === 0 ? (
        <div className={styles.emptyState}>
          <span aria-hidden="true">⌁</span>
          <h2>No paired devices</h2>
          <p>Run <code>fcheap auth login</code> on a trusted machine to pair its first device session.</p>
        </div>
      ) : (
        <>
          <div className={styles.deviceList} id="access-device-list">
            {devices.map((device) => {
              const confirming = confirmingId === device.id;
              const revoking = revokingId === device.id;
              return (
                <article className={styles.device} key={device.id}>
                  <header className={styles.deviceHeader}>
                    <div>
                      <h3>{device.clientName}</h3>
                      <p className={styles.deviceId}>Device {shortId(device.id)}</p>
                    </div>
                    <Status status={device.status} />
                  </header>
                  <dl className={styles.deviceFacts}>
                    <Fact label="Paired" value={device.createdAt} />
                    <Fact label="Last refreshed" value={device.lastRefreshedAt} />
                    <Fact label="Idle expiry" value={device.idleExpiresAt} />
                    <Fact label="Maximum expiry" value={device.absoluteExpiresAt} />
                  </dl>
                  {device.status === "active" ? (
                    <div className={styles.deviceActions}>
                      {confirming ? (
                        <DeviceRevocationConfirmation
                          busy={revoking || loadingMore}
                          clientName={device.clientName}
                          confirmButtonRef={confirmButtonRef}
                          deviceId={device.id}
                          onCancel={() => cancelRevocation(device.id)}
                          onConfirm={() => revoke(device)}
                          revoking={revoking}
                        />
                      ) : (
                        <button
                          className={styles.revokeButton}
                          disabled={loadingMore || revokingId !== null}
                          onClick={() => { setConfirmingId(device.id); setMessage(""); }}
                          ref={(element) => {
                            if (element) revokeTriggerRefs.current.set(device.id, element);
                            else revokeTriggerRefs.current.delete(device.id);
                          }}
                          type="button"
                        >
                          Revoke device
                        </button>
                      )}
                    </div>
                  ) : null}
                </article>
              );
            })}
          </div>
          <div className={styles.loadMore}>
            {canLoadMore ? (
              <button
                aria-controls="access-device-list"
                disabled={loadingMore || revokingId !== null}
                onClick={loadMore}
                type="button"
              >
                {loadingMore ? "Loading devices…" : "Load more devices"}
              </button>
            ) : (
              <p>All available devices are loaded.</p>
            )}
            <p aria-live="polite" className={styles.loadStatus} role="status">
              {loadingMore ? "Loading the next device page." : ""}
            </p>
            {loadError ? <p aria-live="assertive" className={styles.loadError} role="alert">{loadError} Try again.</p> : null}
          </div>
        </>
      )}
      <p aria-live="polite" className={styles.message}>{message}</p>
    </section>
  );
}

export function DeviceRevocationConfirmation({
  busy,
  clientName,
  confirmButtonRef,
  deviceId,
  onCancel,
  onConfirm,
  revoking,
}: {
  busy: boolean;
  clientName: string;
  confirmButtonRef?: Ref<HTMLButtonElement>;
  deviceId: string;
  onCancel: () => void;
  onConfirm: () => void;
  revoking: boolean;
}) {
  const descriptionId = `revoke-device-${deviceId}-description`;
  return (
    <div
      aria-describedby={descriptionId}
      aria-label={`Revoke access for ${clientName}`}
      className={styles.confirmation}
      role="group"
    >
      <p id={descriptionId}>Revoke this device? Its current access and refresh credentials will stop working immediately.</p>
      <div>
        <button
          aria-describedby={descriptionId}
          className={styles.dangerButton}
          disabled={busy}
          onClick={onConfirm}
          ref={confirmButtonRef}
          type="button"
        >
          {revoking ? "Revoking…" : "Confirm revoke"}
        </button>
        <button disabled={busy} onClick={onCancel} type="button">Cancel</button>
      </div>
    </div>
  );
}

function Metric({ label, tone = "default", value }: { label: string; tone?: "attention" | "default" | "quiet"; value: number }) {
  const className = tone === "attention"
    ? `${styles.metric} ${styles.metricAttention}`
    : tone === "quiet"
      ? `${styles.metric} ${styles.metricQuiet}`
      : styles.metric;
  return <div className={className}><dt>{label}</dt><dd>{value}</dd></div>;
}

function Status({ status }: { status: AccessDevice["status"] }) {
  const label = status === "active" ? "Active" : status === "expired" ? "Expired" : "Revoked";
  return <span className={`${styles.status} ${styles[`status${label}`]}`}>{label}</span>;
}

function Fact({ label, value }: { label: string; value: string | null }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value ? <time dateTime={value} suppressHydrationWarning>{formatDate(value)}</time> : "Not refreshed yet"}</dd>
    </div>
  );
}

export function accessDeviceListHref(cursor: string, limit: number): string {
  const params = new URLSearchParams({ cursor, limit: String(limit) });
  return `/api/console/access/devices?${params.toString()}`;
}

export function mergeAccessDevices(
  current: readonly AccessDevice[],
  incoming: readonly AccessDevice[],
): AccessDevice[] {
  const merged = [...current];
  const positions = new Map(merged.map((device, index) => [device.id, index]));
  for (const device of incoming) {
    const position = positions.get(device.id);
    if (position === undefined) {
      positions.set(device.id, merged.length);
      merged.push(device);
      continue;
    }
    const existing = merged[position];
    if (existing?.status !== "revoked" || device.status === "revoked") {
      merged[position] = device;
    }
  }
  return merged;
}

export function appendAccessDevicePage(
  current: AccessDevicePage,
  incoming: AccessDevicePage,
): AccessDevicePage {
  return {
    devices: mergeAccessDevices(current.devices, incoming.devices),
    overview: incoming.overview,
    pageInfo: incoming.pageInfo,
  };
}

export function accessOverviewAfterRevocation(
  overview: AccessDeviceOverview,
  device: AccessDevice,
  now: Date,
): AccessDeviceOverview {
  if (device.status !== "active") return overview;
  const expiresAt = Math.min(
    Date.parse(device.idleExpiresAt),
    Date.parse(device.absoluteExpiresAt),
  );
  const expiring = expiresAt <= now.getTime() + 7 * 24 * 60 * 60 * 1_000;
  return {
    active: Math.max(0, overview.active - 1),
    expiring: expiring ? Math.max(0, overview.expiring - 1) : overview.expiring,
    inactive: Math.min(overview.total, overview.inactive + 1),
    total: overview.total,
  };
}

function problemDetail(payload: unknown, fallback: string): string {
  if (
    payload &&
    typeof payload === "object" &&
    "detail" in payload &&
    typeof payload.detail === "string"
  ) {
    return payload.detail;
  }
  return fallback;
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat("en-US", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value));
}

function shortId(id: string): string {
  return `${id.slice(0, 8)}…${id.slice(-4)}`;
}

export function AccessDashboardLoading() {
  return (
    <section aria-busy="true" aria-label="Loading access controls" className={styles.page}>
      <div className={styles.loadingHeader} />
      <div className={styles.loadingMetrics}>
        {Array.from({ length: 4 }, (_, index) => <div key={index} />)}
      </div>
      <div className={styles.loadingDevices}>
        {Array.from({ length: 3 }, (_, index) => <div key={index} />)}
      </div>
    </section>
  );
}
