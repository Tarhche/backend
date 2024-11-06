import {SkeletonArticleCardVertical} from "@/features/home-page/article-card-vertical/skeleton";

function HashtagsDetailLoading() {
  return [1, 2, 3, 4, 5, 6].map((num) => {
    return <SkeletonArticleCardVertical key={num} />;
  });
}

export default HashtagsDetailLoading;
