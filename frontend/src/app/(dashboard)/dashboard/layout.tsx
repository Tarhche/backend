import {Metadata} from "next";
import {DashboardLayout} from "@/features/dashboard-layout";
import {buildTitle} from "@/lib/seo";

export const metadata: Metadata = {
  title: {
    template: `%s | ${buildTitle("داشبرد", {withBrand: true})}`,
    default: buildTitle("داشبرد"),
  },
};

type Props = {
  children: React.ReactNode;
};

export default function RootLayout({children}: Props) {
  return <DashboardLayout>{children}</DashboardLayout>;
}
