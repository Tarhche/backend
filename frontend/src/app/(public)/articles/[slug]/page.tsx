import {Suspense} from "react";
import {notFound} from "next/navigation";
import {Container, Box} from "@mantine/core";
import {Content, Comments} from "@/features/articles/components/article-detail";

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
      <Suspense fallback={"Loading..."}>
        <Content uuid={slug} />
      </Suspense>
      <Box mt={"xl"}>
        <Suspense fallback={"Loading comments..."}>
          <Comments uuid={slug} />
        </Suspense>
      </Box>
    </Container>
  );
}

export default ArticleDetailPage;
