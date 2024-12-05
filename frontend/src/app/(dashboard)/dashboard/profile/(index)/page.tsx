import {Metadata} from "next";
import {Stack, Paper} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {ProfileUpdateForm} from "@/features/profile/components";
import {fetchUserProfile} from "@/dal/private/profile";

const PAGE_TITLE = "پروفایل";

export const metadata: Metadata = {
  title: PAGE_TITLE,
};

async function UserProfilePage() {
  const user = (await fetchUserProfile()).data;

  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: PAGE_TITLE,
          },
        ]}
      />
      <Paper p="lg" withBorder>
        <ProfileUpdateForm
          userInfo={{
            name: user.name,
            email: user.email,
            username: user.username,
            avatar: user.avatar,
          }}
        />
      </Paper>
    </Stack>
  );
}

export default UserProfilePage;
