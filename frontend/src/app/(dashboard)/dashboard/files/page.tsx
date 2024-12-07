import {type Metadata} from "next";
import {Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {withPermissions} from "@/components/with-authorization";
import {FilesList} from "@/features/files/components";

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

export default withPermissions(FilesPage, {
  requiredPermissions: ["files.index"],
});
