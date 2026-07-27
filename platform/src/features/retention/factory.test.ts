import { afterEach, describe, expect, test } from "bun:test";

import { setArtifactServiceForTests } from "@/features/artifacts/factory";
import {
  getRetentionRunService,
  setRetentionRunServiceForTests,
} from "@/features/retention/factory";
import { RetentionRunService } from "@/features/retention/service";
import { resetDatabaseForTests } from "@/platform/database/client";
import { resetConfigForTests } from "@/shared/config/env";

const original = { ...process.env };

afterEach(() => {
  for (const key of Object.keys(process.env)) if (!(key in original)) delete process.env[key];
  for (const [key, value] of Object.entries(original)) process.env[key] = value;
  resetConfigForTests();
  resetDatabaseForTests();
  setArtifactServiceForTests();
  setRetentionRunServiceForTests();
});

describe("private retention factory", () => {
  test("builds the service without eagerly resolving the plan-receipt keyring", () => {
    // This reproduces the exact misconfiguration that used to make
    // getRetentionRunService() throw synchronously: getArtifactService() was
    // called up front, before the stages array or RetentionRunService
    // existed, so a broken keyring took down every retention stage together.
    process.env.DATABASE_URL = "postgresql://placeholder-config-only@localhost/unused";
    delete process.env.FILECHEAP_PLAN_RECEIPT_ACTIVE_KID;
    delete process.env.FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS;
    delete process.env.FILECHEAP_PLAN_RECEIPT_LOOKUP_KEYS;

    let service: RetentionRunService | undefined;
    expect(() => {
      service = getRetentionRunService();
    }).not.toThrow();
    expect(service).toBeInstanceOf(RetentionRunService);
  });

  test("memoizes the constructed service across calls", () => {
    process.env.DATABASE_URL = "postgresql://placeholder-config-only@localhost/unused";
    delete process.env.FILECHEAP_PLAN_RECEIPT_ACTIVE_KID;
    delete process.env.FILECHEAP_PLAN_RECEIPT_SIGNING_KEYS;
    delete process.env.FILECHEAP_PLAN_RECEIPT_LOOKUP_KEYS;

    const first = getRetentionRunService();
    const second = getRetentionRunService();
    expect(second).toBe(first);
  });
});
