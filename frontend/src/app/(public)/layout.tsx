import type {Metadata} from "next";
import {AppMainShell} from "@/components/app-main-shell";
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
  return <AppMainShell>{children}</AppMainShell>;
}
