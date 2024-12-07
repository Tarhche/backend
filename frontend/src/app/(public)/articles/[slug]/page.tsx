import {type Metadata} from "next";
import {Suspense} from "react";
import {notFound} from "next/navigation";
import {Container, Box, Group, Title} from "@mantine/core";
import {IconMessage} from "@tabler/icons-react";
import {
  Content,
  ContentSkeleton,
  Comments,
  CommentsSkeleton,
} from "@/features/articles/components/article-detail";
import {fetchArticleByUUID} from "@/dal/public/articles";

type Props = {
  params: {
    slug?: string;
  };
};

export async function generateMetadata({
  params,
}: Props): Promise<Metadata | null> {
  const slug = params.slug;
  if (slug === undefined) {
    return null;
  }
  const article = await fetchArticleByUUID(slug);
  return {
    title: `${article.title}`,
  };
}

async function ArticleDetailPage({params: {slug}}: Props) {
  if (slug === undefined) {
    notFound();
  }

  return (
    <Container size={"sm"} mt={"xl"} component="section">
      <Suspense fallback={<ContentSkeleton />}>
        <Content uuid={slug} />
      </Suspense>
      <Box mt={"xl"}>
        <Group align="center" gap={"sm"}>
          <IconMessage />
          <Title ta={"right"} order={3}>
            دیدگاه ها
          </Title>
        </Group>
        <Suspense fallback={<CommentsSkeleton />}>
          <Comments uuid={slug} />
        </Suspense>
      </Box>
    </Container>
  );
}

export default ArticleDetailPage;
