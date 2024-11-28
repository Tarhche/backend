import {Metadata} from "next";
import {Suspense} from "react";
import {Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {
  RolesTable,
  RolesTableSkeleton,
} from "@/features/roles/components/roles-table";

const PAGE_TITLE = "نقش ها";

export const metadata: Metadata = {
  title: PAGE_TITLE,
};

type Props = {
  searchParams: {
    page?: string;
  };
};

function RolesPage({searchParams}: Props) {
  const page = searchParams.page ?? 1;

  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: PAGE_TITLE,
          },
        ]}
      />
      <Suspense key={page} fallback={<RolesTableSkeleton />}>
        <RolesTable page={page} />
      </Suspense>
    </Stack>
  );
}

export default RolesPage;
