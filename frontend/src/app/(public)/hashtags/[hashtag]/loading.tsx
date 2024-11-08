import {VerticalArticleCardSkeleton} from "@/features/home-page/components/article-card-vertical";

function HashtagsDetailLoading() {
  return [1, 2, 3, 4, 5, 6].map((num) => {
    return <VerticalArticleCardSkeleton key={num} />;
  });
}

export default HashtagsDetailLoading;
