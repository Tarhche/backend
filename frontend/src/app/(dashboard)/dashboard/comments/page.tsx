import {type Metadata} from "next";
import {Suspense} from "react";
import {Box} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {
  CommentsTable,
  CommentsTableSkeleton,
} from "@/features/dashboard/components/comments";
import {APP_PATHS} from "@/lib/app-paths";

export const metadata: Metadata = {
  title: "کامنت کاربران",
};

type Props = {
  searchParams: {
    page?: string;
  };
};

async function CommentsPage({searchParams}: Props) {
  const {page} = searchParams;

  return (
    <Box>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "کامنت کاربران",
            href: APP_PATHS.dashboard.comments.index,
          },
        ]}
      />
      <Box mt={"md"}>
        <Suspense
          key={JSON.stringify(searchParams)}
          fallback={<CommentsTableSkeleton />}
        >
          <CommentsTable page={page ?? 1} />
        </Suspense>
      </Box>
    </Box>
  );
}

export default CommentsPage;
