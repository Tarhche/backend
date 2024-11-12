import {Metadata} from "next";
import {Suspense} from "react";
import {Box, Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {
  MyCommentsTable,
  MyCommentsTableSkeleton,
} from "@/features/dashboard/components/my-comments";

export const metadata: Metadata = {
  title: "کامنت های من",
};

type Props = {
  searchParams: {
    page?: string;
  };
};

function MyCommentsPage({searchParams}: Props) {
  const page = Number(searchParams.page) || 1;

  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "کامنت های من",
          },
        ]}
      />
      <Box>
        <Suspense
          key={JSON.stringify(searchParams)}
          fallback={<MyCommentsTableSkeleton />}
        >
          <MyCommentsTable page={page} />
        </Suspense>
      </Box>
    </Stack>
  );
}

export default MyCommentsPage;
