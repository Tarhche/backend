import {Metadata} from "next";
import {Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {APP_PATHS} from "@/lib/app-paths";

type Props = {
  children: React.ReactNode;
};

const PAGE_TITLE = "ایجاد نقش";

export const metadata: Metadata = {
  title: PAGE_TITLE,
};

function Layout({children}: Props) {
  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "نقش ها",
            href: APP_PATHS.dashboard.roles.index,
          },
          {
            label: PAGE_TITLE,
          },
        ]}
      />
      {children}
    </Stack>
  );
}

export default Layout;
