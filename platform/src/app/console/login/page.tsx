import type { Metadata } from "next";

import { AuthFlow } from "@/features/console/auth/AuthFlow";

export const metadata: Metadata = { title: "Sign in" };

export default function ConsoleLoginPage() {
  return <AuthFlow mode="login" />;
}
