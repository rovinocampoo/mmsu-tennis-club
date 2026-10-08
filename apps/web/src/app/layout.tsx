import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "MMSU Tennis Club",
  description: "The official web home of the MMSU Tennis Club.",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
