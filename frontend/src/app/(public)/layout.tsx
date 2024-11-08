import {AppMainShell} from "@/components/app-main-shell";

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return <AppMainShell>{children}</AppMainShell>;
}
