import {Metadata} from "next";
import {Suspense} from "react";
import {Box, Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {
  MyBookmarksTable,
  MyBookmarksTableSkeleton,
} from "@/features/dashboard/components/my-bookmarks";

const title = "بوکمارک های من";

export const metadata: Metadata = {
  title: title,
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
            label: title,
          },
        ]}
      />
      <Box>
        <Suspense
          key={JSON.stringify(searchParams)}
          fallback={<MyBookmarksTableSkeleton />}
        >
          <MyBookmarksTable page={page} />
        </Suspense>
      </Box>
    </Stack>
  );
}

export default MyBookmarksPage;
