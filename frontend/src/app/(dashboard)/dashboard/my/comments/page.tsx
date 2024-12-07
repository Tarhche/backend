import {Metadata} from "next";
import {Suspense} from "react";
import {Box, Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {
  UserCommentsTable,
  UserCommentsTableSkeleton,
} from "@/features/comments/components/user-comments-table";
import {withPermissions} from "@/components/with-authorization";

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
          fallback={<UserCommentsTableSkeleton />}
        >
          <UserCommentsTable page={page} />
        </Suspense>
      </Box>
    </Stack>
  );
}

export default withPermissions(MyCommentsPage, {
  requiredPermissions: ["self.comments.index"],
});
