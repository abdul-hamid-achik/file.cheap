import { PlatformError } from "@/shared/errors/platform-error";

type RecoveryLabEnvironment = {
  NODE_ENV?: string;
  PLATFORM_RECOVERY_LAB_ENABLED?: string;
  VERCEL?: string;
  VERCEL_ENV?: string;
};

export function isRecoveryLabEnabled(
  environment: RecoveryLabEnvironment = process.env,
): boolean {
  const configuredValue = environment.PLATFORM_RECOVERY_LAB_ENABLED;
  if (configuredValue !== undefined && configuredValue !== "true") {
    return false;
  }

  // Vercel Preview uses NODE_ENV=production too. Only an exact known
  // non-Production target with an explicit opt-in may override that default;
  // Production and unexpected deployment signals fail closed.
  if (environment.VERCEL_ENV !== undefined) {
    return (
      configuredValue === "true" &&
      ["preview", "development"].includes(environment.VERCEL_ENV)
    );
  }

  if (environment.NODE_ENV === "production" || Boolean(environment.VERCEL)) {
    return false;
  }

  return configuredValue === undefined || configuredValue === "true";
}

export function requireRecoveryLabAccess(
  environment: RecoveryLabEnvironment = process.env,
): void {
  if (isRecoveryLabEnabled(environment)) {
    return;
  }

  throw new PlatformError({
    code: "route_unavailable",
    detail: "The experimental recovery lab is disabled for this deployment.",
    status: 404,
    title: "Route unavailable",
  });
}
