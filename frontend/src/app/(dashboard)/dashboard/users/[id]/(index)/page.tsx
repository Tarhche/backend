import {Metadata} from "next";
import {notFound} from "next/navigation";
import {Box, Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {UpsertUserForm} from "@/features/users/components/upsert-user-form";
import {withPermissions} from "@/components/with-authorization";
import {fetchUser} from "@/dal/private/users";
import {APP_PATHS} from "@/lib/app-paths";

const PAGE_TITLE = "بروزرسانی کاربر";

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
            label: PAGE_TITLE,
          },
        ]}
      />
      <Box>
        <UpsertUserForm
          userInfo={{
            userId: userData.uuid,
            defaultAvatar: userData.avatar,
            defaultName: userData.name,
            defaultEmail: userData.email,
            defaultUsername: userData.username,
          }}
        />
      </Box>
    </Stack>
  );
}

export default withPermissions(UpdateUserPage, {
  requiredPermissions: ["users.show", "users.update"],
  operator: "AND",
});
