import {Metadata} from "next";
import {Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {RolesTable} from "@/features/dashboard/roles-table";

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
      <RolesTable page={page} />
    </Stack>
  );
}

export default RolesPage;
