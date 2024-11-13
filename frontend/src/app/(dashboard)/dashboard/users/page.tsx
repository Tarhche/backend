import {Metadata} from "next";
import {Suspense} from "react";
import {Box, Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {
  UsersTable,
  UsersTableSkeleton,
} from "@/features/dashboard/components/users";

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

export default MyBookmarksPage;
