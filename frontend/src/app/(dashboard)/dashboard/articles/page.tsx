import {type Metadata} from "next";
import {Box} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {APP_PATHS} from "@/lib/app-paths";

export const metadata: Metadata = {
  title: "مقاله ها",
};

export default function ArticlesPage() {
  return (
    <Box>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "مقاله ها",
            href: APP_PATHS.dashboard.articles,
          },
        ]}
      />
    </Box>
  );
}
