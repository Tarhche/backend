import {type Metadata} from "next";
import {Box, Paper} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {ArticleUpsertForm} from "@/features/articles/components/article-upsert-form";
import {withPermissions} from "@/components/with-authorization";
import {APP_PATHS} from "@/lib/app-paths";

export const metadata: Metadata = {
  title: "مقاله جدید",
};

async function NewArticlesPage() {
  return (
    <Box>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "مقاله ها",
            href: APP_PATHS.dashboard.articles.index,
          },
          {
            label: "مقاله جدید",
          },
        ]}
      />
      <Paper p={"md"} mt={"md"} withBorder>
        <ArticleUpsertForm />
      </Paper>
    </Box>
  );
}

export default withPermissions(NewArticlesPage, {
  requiredPermissions: ["articles.create"],
});
