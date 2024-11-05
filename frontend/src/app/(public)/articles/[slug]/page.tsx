import {Suspense} from "react";
import {notFound} from "next/navigation";
import {Container, Box, Group, Title} from "@mantine/core";
import {IconMessage} from "@tabler/icons-react";
import {
  Content,
  ContentSkeleton,
  Comments,
} from "@/features/articles/components/article-detail";

type Props = {
  params: {
    slug?: string;
  };
};

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
        <Suspense fallback={"Loading comments..."}>
          <Comments uuid={slug} />
        </Suspense>
      </Box>
    </Container>
  );
}

export default ArticleDetailPage;
