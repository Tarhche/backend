import {notFound} from "next/navigation";
import {Stack, Paper} from "@mantine/core";
import {ArticleUpsertForm} from "@/features/dashboard/article-upsert-form";
import {DashboardBreadcrumbs} from "@/features/dashboard/components/breadcrumbs";
import {fetchArticle} from "@/dal";
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

export default ArticleDetalPage;
