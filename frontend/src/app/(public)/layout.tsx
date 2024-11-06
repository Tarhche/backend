import {AppMainShell} from "@/components/app-main-shell";
import "@mantine/core/styles.css";

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return <AppMainShell>{children}</AppMainShell>;
}
