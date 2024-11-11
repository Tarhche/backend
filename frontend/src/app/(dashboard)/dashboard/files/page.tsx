import {type Metadata} from "next";
import {Suspense} from "react";
import {Stack} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {
  FilesList,
  FilesListSkeleton,
} from "@/features/dashboard/components/files";

export const metadata: Metadata = {
  title: "فایل ها",
};

type Props = {
  searchParams: {
    page?: number;
  };
};

async function FilesPage({searchParams}: Props) {
  const page = searchParams.page ?? 1;

  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "فایل ها",
          },
        ]}
      />
      <Suspense
        key={JSON.stringify(searchParams ?? {})}
        fallback={<FilesListSkeleton />}
      >
        <FilesList page={page} />
      </Suspense>
    </Stack>
  );
}

export default FilesPage;
