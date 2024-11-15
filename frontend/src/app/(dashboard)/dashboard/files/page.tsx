import {type Metadata} from "next";
import {Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {FilesList} from "@/features/dashboard/components/files";

export const metadata: Metadata = {
  title: "فایل ها",
};

async function FilesPage() {
  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "فایل ها",
          },
        ]}
      />
      <FilesList />
    </Stack>
  );
}

export default FilesPage;
