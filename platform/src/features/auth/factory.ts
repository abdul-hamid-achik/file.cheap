import { AuthService } from "@/features/auth/service";
import { DrizzleAuthRepository } from "@/platform/database/auth-repository";
import { ResendAuthMailer } from "@/platform/mail/resend-auth-mailer";
import { getAuthConfig } from "@/shared/config/auth";

let service: AuthService | undefined;

export function getAuthService(): AuthService {
  if (!service) {
    const config = getAuthConfig();
    service = new AuthService(
      new DrizzleAuthRepository(),
      new ResendAuthMailer(config.resendApiKey, config.from),
      {
        allowedEmails: config.allowedEmails,
        ownerAccountId: config.ownerAccountId,
        publicUrl: config.publicUrl,
        secret: config.secret,
        verificationDeliveryLeaseMs: config.verificationDeliveryLeaseMs,
      },
    );
  }
  return service;
}

export function setAuthServiceForTests(value?: AuthService): void {
  service = value;
}
