import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";

import type {
  AccessDevice,
  AccessDeviceListResponse,
} from "@/features/auth/contracts";

import {
  AccessDashboard,
  DeviceRevocationConfirmation,
  accessDeviceListHref,
  accessOverviewAfterRevocation,
  appendAccessDevicePage,
  mergeAccessDevices,
} from "./AccessDashboard";

const device: AccessDevice = {
  absoluteExpiresAt: "2026-10-24T12:00:00.000Z",
  clientName: "Build Mac",
  createdAt: "2026-07-26T12:00:00.000Z",
  id: "2b5445d0-2c84-4eef-a222-ab6618b9c803",
  idleExpiresAt: "2026-08-25T12:00:00.000Z",
  lastRefreshedAt: "2026-07-26T12:05:00.000Z",
  revokedAt: null,
  status: "active",
};

const initialPage = {
  devices: [device],
  overview: { active: 12, expiring: 3, inactive: 27, total: 42 },
  pageInfo: { endCursor: "cursor_next", hasNextPage: true, limit: 20 },
} satisfies Omit<AccessDeviceListResponse, "version">;

describe("AccessDashboard", () => {
  test("renders exact server overview, bounded loaded count, and load-more affordance", () => {
    const html = renderToStaticMarkup(
      <AccessDashboard initialPage={initialPage} now={new Date("2026-07-26T12:10:00.000Z")} />,
    );
    expect(html).toContain("Build Mac");
    expect(html).toContain("Last refreshed");
    expect(html).toContain("Revoke device");
    expect(html).toContain("<dt>Total paired</dt><dd>42</dd>");
    expect(html).toContain("<dt>Active devices</dt><dd>12</dd>");
    expect(html).toContain("<dt>Expiring in 7 days</dt><dd>3</dd>");
    expect(html).toContain("<dt>Revoked or expired</dt><dd>27</dd>");
    expect(html).toContain("<strong>1</strong> of 42 loaded");
    expect(html).toContain("Load more devices");
    expect(html).toContain('aria-controls="access-device-list"');
    expect(html).toContain('aria-live="polite"');
    expect(html).not.toContain("token_hash");
    expect(html).not.toContain("refreshToken");
  });

  test("builds the next-page request from the opaque cursor and current limit", () => {
    expect(accessDeviceListHref("cursor.with/signature", 50)).toBe(
      "/api/console/access/devices?cursor=cursor.with%2Fsignature&limit=50",
    );
  });

  test("appends unique devices without allowing a stale page to undo revocation", () => {
    const revoked = {
      ...device,
      revokedAt: "2026-07-26T12:15:00.000Z",
      status: "revoked" as const,
    };
    const second = {
      ...device,
      clientName: "Travel Mac",
      id: "8c4bc20d-ec8a-470e-b0fd-2582d7704567",
    };

    expect(mergeAccessDevices([revoked], [device, second])).toEqual([
      revoked,
      second,
    ]);

    const next = appendAccessDevicePage(
      { ...initialPage, devices: [revoked] },
      {
        devices: [device, second],
        overview: { active: 11, expiring: 2, inactive: 28, total: 42 },
        pageInfo: { endCursor: null, hasNextPage: false, limit: 20 },
      },
    );
    expect(next.devices).toEqual([revoked, second]);
    expect(next.overview).toEqual({ active: 11, expiring: 2, inactive: 28, total: 42 });
    expect(next.pageInfo).toEqual({ endCursor: null, hasNextPage: false, limit: 20 });
  });

  test("updates the exact overview when an expiring active device is revoked", () => {
    expect(accessOverviewAfterRevocation(
      initialPage.overview,
      { ...device, idleExpiresAt: "2026-07-29T12:00:00.000Z" },
      new Date("2026-07-26T12:00:00.000Z"),
    )).toEqual({ active: 11, expiring: 2, inactive: 28, total: 42 });
  });

  test("describes revocation from the confirm control without an eager alert", () => {
    const html = renderToStaticMarkup(
      <DeviceRevocationConfirmation
        busy={false}
        clientName={device.clientName}
        deviceId={device.id}
        onCancel={() => undefined}
        onConfirm={() => undefined}
        revoking={false}
      />,
    );

    const descriptionId = `revoke-device-${device.id}-description`;
    expect(html).toContain('role="group"');
    expect(html).toContain('aria-label="Revoke access for Build Mac"');
    expect(html).toContain(`aria-describedby="${descriptionId}"`);
    expect(html).toContain(`id="${descriptionId}"`);
    expect(html).not.toContain('role="alert"');
  });
});
