import type { Metadata } from "next";

import { AuthFlow } from "@/features/console/auth/AuthFlow";

export const metadata: Metadata = { title: "Approve device" };

export default async function ConsoleActivatePage({ searchParams }: { searchParams: Promise<{ code?: string }> }) {
  const { code } = await searchParams;
  return <AuthFlow initialUserCode={code} mode="activate" />;
}
