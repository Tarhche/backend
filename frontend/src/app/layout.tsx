import type {Metadata} from "next";
import {ColorSchemeScript} from "@mantine/core";
import {AppMainShell} from "@/components/app-main-shell";
import {Providers} from "./providers";
import {vazir} from "./fonts";
import "@mantine/core/styles.css";

export const metadata: Metadata = {
  title: "طرح چه",
  description: "طرح‌چه",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="fa" dir="rtl">
      <head>
        <ColorSchemeScript />
      </head>
      <body className={`${vazir.className} antialiased`}>
        <Providers>
          <AppMainShell>{children}</AppMainShell>
        </Providers>
      </body>
    </html>
  );
}
