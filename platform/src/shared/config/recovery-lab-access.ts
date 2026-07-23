import { PlatformError } from "@/shared/errors/platform-error";

type RecoveryLabEnvironment = {
  NODE_ENV?: string;
  PLATFORM_RECOVERY_LAB_ENABLED?: string;
  VERCEL?: string;
};

export function isRecoveryLabEnabled(
  environment: RecoveryLabEnvironment = process.env,
): boolean {
  const configuredValue = environment.PLATFORM_RECOVERY_LAB_ENABLED;
  if (configuredValue !== undefined) {
    return configuredValue === "true";
  }

  const productionEnvironment =
    environment.NODE_ENV === "production" || Boolean(environment.VERCEL);

  if (productionEnvironment) {
    return false;
  }

  return true;
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
