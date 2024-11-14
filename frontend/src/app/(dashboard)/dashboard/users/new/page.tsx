import {Metadata} from "next";
import {Box, Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {UpsertUserForm} from "@/features/dashboard/components/upsert-user-form";
import {APP_PATHS} from "@/lib/app-paths";

const PAGE_TITLE = "کاربر جدید";

export const metadata: Metadata = {
  title: PAGE_TITLE,
};

function NewUserPage() {
  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "کاربرها",
            href: APP_PATHS.dashboard.users.index,
          },
          {
            label: PAGE_TITLE,
          },
        ]}
      />
      <Box>
        <UpsertUserForm />
      </Box>
    </Stack>
  );
}

export default NewUserPage;
