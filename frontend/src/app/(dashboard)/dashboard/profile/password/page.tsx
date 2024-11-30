import {Metadata} from "next";
import {Stack} from "@mantine/core";
import {Paper} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {ProfilePasswordForm} from "@/features/profile/components";
import {APP_PATHS} from "@/lib/app-paths";

const PAGE_TITLE = "تغییر کلمه عبور";

export const metadata: Metadata = {
  title: PAGE_TITLE,
};

async function ChangePasswordPage() {
  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "پروفایل",
            href: APP_PATHS.dashboard.profile.index,
          },
          {
            label: PAGE_TITLE,
          },
        ]}
      />
      <Paper p="lg" withBorder>
        <ProfilePasswordForm />
      </Paper>
    </Stack>
  );
}

export default ChangePasswordPage;
