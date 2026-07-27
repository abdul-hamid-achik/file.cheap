import { afterEach, describe, expect, test } from "bun:test";

import { DELETE as revokeDevice } from "@/app/api/console/access/devices/[familyId]/route";
import { GET as listDevices } from "@/app/api/console/access/devices/route";
import { setAuthServiceForTests } from "@/features/auth/factory";
import { InMemoryAuthRepository } from "@/features/auth/repository";
import { AuthService, type AuthMailer } from "@/features/auth/service";

class CapturingMailer implements AuthMailer {
  messages: Parameters<AuthMailer["sendVerification"]>[0][] = [];
  async sendVerification(input: Parameters<AuthMailer["sendVerification"]>[0]) {
    this.messages.push(input);
  }
}

afterEach(() => setAuthServiceForTests());

describe("console access device routes", () => {
  test("lists only with a web session and rejects a device bearer", async () => {
    const fixture = await accessFixture();
    const response = await listDevices(new Request("https://file.cheap/api/console/access/devices", {
      headers: { cookie: `fcheap_session=${fixture.webToken}` },
    }));
    expect(response.status).toBe(200);
    expect(await response.json()).toMatchObject({
      devices: [{ clientName: "Build Mac", status: "active" }],
      overview: { active: 1, expiring: 0, inactive: 0, total: 1 },
      pageInfo: { hasNextPage: false, limit: 20 },
      version: "filecheap-access/1",
    });

    const bearerResponse = await listDevices(new Request("https://file.cheap/api/console/access/devices", {
      headers: { authorization: `Bearer ${fixture.deviceToken}` },
    }));
    expect(bearerResponse.status).toBe(401);
  });

  test("validates cursor and limit query parameters", async () => {
    const fixture = await accessFixture();
    const headers = { cookie: `fcheap_session=${fixture.webToken}` };

    const bounded = await listDevices(new Request(
      "https://file.cheap/api/console/access/devices?limit=1",
      { headers },
    ));
    expect(bounded.status).toBe(200);
    expect((await bounded.json()).pageInfo).toMatchObject({ limit: 1 });

    const invalidCursor = await listDevices(new Request(
      "https://file.cheap/api/console/access/devices?cursor=not-a-signed-cursor",
      { headers },
    ));
    expect(invalidCursor.status).toBe(422);
    expect((await invalidCursor.json()).code).toBe("invalid_cursor");

    for (const query of ["limit=0", "limit=51", "limit=invalid"]) {
      const invalid = await listDevices(new Request(
        `https://file.cheap/api/console/access/devices?${query}`,
        { headers },
      ));
      expect(invalid.status).toBe(422);
      expect((await invalid.json()).code).toBe("invalid_request");
    }
  });

  test("rejects cross-origin deletion before auth and reports malformed IDs as validation errors", async () => {
    const fixture = await accessFixture();
    const crossOrigin = await revokeDevice(
      new Request(`https://file.cheap/api/console/access/devices/${fixture.familyId}`, {
        headers: { origin: "https://attacker.example", "sec-fetch-site": "cross-site" },
        method: "DELETE",
      }),
      { params: Promise.resolve({ familyId: fixture.familyId }) },
    );
    expect(crossOrigin.status).toBe(403);

    const malformed = await revokeDevice(
      new Request("https://file.cheap/api/console/access/devices/not-a-uuid", {
        headers: {
          cookie: `fcheap_session=${fixture.webToken}`,
          origin: "https://file.cheap",
          "sec-fetch-site": "same-origin",
        },
        method: "DELETE",
      }),
      { params: Promise.resolve({ familyId: "not-a-uuid" }) },
    );
    expect(malformed.status).toBe(422);
    expect((await malformed.json()).code).toBe("invalid_request");
  });
});

async function accessFixture(): Promise<{ deviceToken: string; familyId: string; webToken: string }> {
  const repository = new InMemoryAuthRepository();
  const mailer = new CapturingMailer();
  const service = new AuthService(repository, mailer, {
    allowedEmails: ["owner@example.com"],
    publicUrl: "https://file.cheap",
    secret: "s".repeat(32),
  });

  const deviceAuthorization = await service.startDeviceAuthorization({
    clientName: "Build Mac",
    clientType: "agent",
  });
  await service.sendVerification({ email: "owner@example.com", userCode: deviceAuthorization.userCode });
  await service.decide({
    decision: "approve",
    email: "owner@example.com",
    otp: mailer.messages.at(-1)!.otp,
    userCode: deviceAuthorization.userCode,
  });
  const deviceCredentials = await service.poll(deviceAuthorization.deviceCode);

  const browserAuthorization = await service.startDeviceAuthorization({
    clientName: "Web console",
    clientType: "browser",
  });
  await service.sendVerification({ email: "owner@example.com", userCode: browserAuthorization.userCode });
  const browserSession = await service.decide({
    decision: "approve",
    email: "owner@example.com",
    otp: mailer.messages.at(-1)!.otp,
    userCode: browserAuthorization.userCode,
  });
  if (!browserSession?.sessionToken) throw new Error("Expected a browser session");
  const familyId = (await service.listAccessDevices(
    browserSession.userId,
    { limit: 20 },
  )).devices[0]?.id;
  if (!familyId) throw new Error("Expected a paired device family");
  setAuthServiceForTests(service);
  return {
    deviceToken: deviceCredentials.accessToken,
    familyId,
    webToken: browserSession.sessionToken,
  };
}
