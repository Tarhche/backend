import {type Metadata} from "next";
import {Suspense} from "react";
import {Box} from "@mantine/core";
import {withPermissions} from "@/components/with-authorization";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {
  ArticlesTable,
  ArticlesTableSkeleton,
} from "@/features/articles/components/articles-table";
import {APP_PATHS} from "@/lib/app-paths";

export const metadata: Metadata = {
  title: "مقاله ها",
};

type Props = {
  searchParams: {
    page?: string;
  };
};

async function ArticlesPage({searchParams}: Props) {
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
      <Box py="md">
        <Suspense key={page} fallback={<ArticlesTableSkeleton />}>
          <ArticlesTable page={page ?? 1} />
        </Suspense>
      </Box>
    </Box>
  );
}

export default withPermissions(ArticlesPage, {
  requiredPermissions: ["articles.index"],
});
