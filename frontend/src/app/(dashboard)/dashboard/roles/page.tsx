import {Metadata} from "next";
import {Suspense} from "react";
import {Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {RolesTable, RolesTableSkeleton} from "@/features/roles/components";
import {withPermissions} from "@/components/with-authorization";

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

export default withPermissions(RolesPage, {
  requiredPermissions: ["roles.index"],
});
