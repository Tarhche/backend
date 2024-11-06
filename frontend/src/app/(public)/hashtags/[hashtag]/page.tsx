import {notFound} from "next/navigation";
import {ArticleCardVertical} from "@/features/home-page/article-card-vertical";
import {fetchAllArticlesByHashtag} from "@/dal/hashtags";

type Props = {
  params: {
    hashtag?: string;
  };
};

async function HashtagPage({params}: Props) {
  const hashtag = params.hashtag;
  if (hashtag === undefined) {
    notFound();
  }
  const articles = (await fetchAllArticlesByHashtag(hashtag)).items;

  return articles.map((article: any) => {
    return (
      <ArticleCardVertical
        key={article.uuid}
        article={{
          thumbnail: article.cover,
          title: article.title,
          subtitle: article.excerpt,
          publishedDate: article.published_at,
          slug: article.uuid,
        }}
      />
    );
  });
}

export default HashtagPage;
