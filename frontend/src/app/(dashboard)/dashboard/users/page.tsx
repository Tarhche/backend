import {Metadata} from "next";
import {Suspense} from "react";
import {Box, Stack} from "@mantine/core";
import {withPermissions} from "@/components/with-authorization";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {UsersTable, UsersTableSkeleton} from "@/features/users/components";

const PAGE_TITLE = "کاربر ها";

export const metadata: Metadata = {
  title: PAGE_TITLE,
};

type Props = {
  searchParams: {
    page?: string;
  };
};

function MyBookmarksPage({searchParams}: Props) {
  const page = Number(searchParams.page) || 1;

  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: PAGE_TITLE,
          },
        ]}
      />
      <Box>
        <Suspense
          key={JSON.stringify(searchParams)}
          fallback={<UsersTableSkeleton />}
        >
          <UsersTable page={page} />
        </Suspense>
      </Box>
    </Stack>
  );
}

export default withPermissions(MyBookmarksPage, {
  requiredPermissions: ["users.index"],
});
