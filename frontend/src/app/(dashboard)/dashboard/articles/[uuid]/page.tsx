import {notFound} from "next/navigation";
import {Stack, Paper} from "@mantine/core";
import {ArticleUpsertForm} from "@/features/articles/components/article-upsert-form";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {withPermissions} from "@/components/with-authorization";
import {fetchArticle} from "@/dal/private/articles";
import {APP_PATHS} from "@/lib/app-paths";

export const metadata = {
  title: "جزییات مقاله",
};

type Props = {
  params: {
    uuid?: string;
  };
};

async function ArticleDetalPage({params}: Props) {
  const articleId = params.uuid;
  if (articleId === undefined) {
    notFound();
  }

  const article = await fetchArticle(articleId);

  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: "مقاله ها",
            href: APP_PATHS.dashboard.articles.index,
          },
          {
            label: "ویرایش مقاله",
          },
        ]}
      />
      <Paper p="md" withBorder>
        <ArticleUpsertForm
          article={{
            defaultTitle: article.title,
            articleId: article.uuid,
            defaultExcerpt: article.excerpt,
            defaultHashtags: article.tags,
            defaultBody: article.body,
            defaultCover: article.cover,
            defaultVideo: article.video,
            defaultPublishedAt: article.published_at,
          }}
        />
      </Paper>
    </Stack>
  );
}

export default withPermissions(ArticleDetalPage, {
  requiredPermissions: ["articles.show", "articles.update"],
  operator: "AND",
});
