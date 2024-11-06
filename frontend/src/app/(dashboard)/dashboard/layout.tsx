import {Metadata} from "next";
import {type ReactNode} from "react";
import {DashboardLayout} from "@/features/dashboard/dashbaord-layout";

export const metadata: Metadata = {
  title: "داشبرد",
};

type Props = {
  children: ReactNode;
};

export default function RootLayout({children}: Props) {
  return <DashboardLayout>{children}</DashboardLayout>;
}
