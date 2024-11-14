import {Metadata} from "next";
import {notFound} from "next/navigation";
import {Box, Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {UserPasswordForm} from "@/features/dashboard/components/change-user-password";
import {fetchUser} from "@/dal";
import {APP_PATHS} from "@/lib/app-paths";

const PAGE_TITLE = "تغییر کلمه عبور کاربر";

export const metadata: Metadata = {
  title: PAGE_TITLE,
};

type Props = {
  params: {
    id?: string;
  };
};

async function UpdateUserPage({params}: Props) {
  const userId = params.id;
  if (userId === undefined) {
    notFound();
  }
  const userData = await fetchUser(userId);

  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "کاربرها",
            href: APP_PATHS.dashboard.users.index,
          },
          {
            label: userData.name,
            href: APP_PATHS.dashboard.users.edit(userData.uuid),
          },
          {
            label: PAGE_TITLE,
          },
        ]}
      />
      <Box>
        <UserPasswordForm userId={userId} />
      </Box>
    </Stack>
  );
}

export default UpdateUserPage;
