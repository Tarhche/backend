import {Suspense} from "react";
import {notFound} from "next/navigation";
import {Container, Box} from "@mantine/core";
import {ArticleDetail} from "@/features/articles/components/article-detail";
import {Comments} from "@/features/articles/components/comments";

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
    <Container size={"sm"} mt={"xl"}>
      <Suspense fallback={"Loading..."}>
        <ArticleDetail uuid={slug} />
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
