import {type Metadata} from "next";
import {Suspense} from "react";
import {Box} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {ArticlesTable} from "@/features/dashboard/components/articles/articles-table";
import {ArticlesTableSkeleton} from "@/features/dashboard/components/articles/articles-table-skeleton";
import {APP_PATHS} from "@/lib/app-paths";

export const metadata: Metadata = {
  title: "مقاله ها",
};

type Props = {
  searchParams: {
    page?: string;
  };
};

export default async function ArticlesPage({searchParams}: Props) {
  const {page} = searchParams;

  return (
    <Box>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "مقاله ها",
            href: APP_PATHS.dashboard.articles.index,
          },
        ]}
      />
      <Box mt={"md"}>
        <Suspense
          key={JSON.stringify(searchParams)}
          fallback={<ArticlesTableSkeleton />}
        >
          <ArticlesTable page={page ?? 1} />
        </Suspense>
      </Box>
    </Box>
  );
}
