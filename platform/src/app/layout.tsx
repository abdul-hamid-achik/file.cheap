import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono } from "next/font/google";

import "./globals.css";

const geistSans = Geist({
  display: "swap",
  subsets: ["latin"],
  variable: "--font-geist-sans",
});

const geistMono = Geist_Mono({
  display: "swap",
  subsets: ["latin"],
  variable: "--font-geist-mono",
});

export const metadata: Metadata = {
  applicationName: "file.cheap",
  description:
    "A local-first CLI and MCP server for saving, searching, verifying, and restoring agent-generated files.",
  icons: {
    icon: "/favicon.svg",
  },
  metadataBase: new URL("https://file.cheap"),
  title: {
    default: "file.cheap — local artifact vault for coding agents",
    template: "%s — file.cheap",
  },
};

export const viewport: Viewport = {
  colorScheme: "dark",
  themeColor: "#181713",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" className={`${geistSans.variable} ${geistMono.variable}`}>
      <body>{children}</body>
    </html>
  );
}
