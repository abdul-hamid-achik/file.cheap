import type { Metadata, Viewport } from "next";

import "./globals.css";

export const metadata: Metadata = {
  description:
    "Recovery-first lab for the optional file.cheap verified remote vault protocol.",
  robots: {
    follow: false,
    index: false,
  },
  title: {
    default: "Verified remote vault lab — file.cheap",
    template: "%s — file.cheap",
  },
};

export const viewport: Viewport = {
  colorScheme: "dark",
  themeColor: "#181713",
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
