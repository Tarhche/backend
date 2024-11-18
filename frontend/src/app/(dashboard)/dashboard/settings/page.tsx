import {Metadata} from "next";
import {Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";

const PAGE_TITLE = "تنظیمات";

export const metadata: Metadata = {
  title: PAGE_TITLE,
};

function SettingsPage() {
  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: PAGE_TITLE,
          },
        ]}
      />
    </Stack>
  );
}

export default SettingsPage;
