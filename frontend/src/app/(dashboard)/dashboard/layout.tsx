import {Metadata} from "next";
import {type ReactNode} from "react";
import {DashboardLayout} from "@/features/dashboard/dashbaord-layout";
import {buildTitle} from "@/lib/seo";

export const metadata: Metadata = {
  title: {
    template: `%s | ${buildTitle("داشبرد", {withBrand: true})}`,
    default: buildTitle("داشبرد"),
  },
};

type Props = {
  children: ReactNode;
};

export default function RootLayout({children}: Props) {
  return <DashboardLayout>{children}</DashboardLayout>;
}
