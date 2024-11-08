import {type Metadata} from "next";
import {Suspense} from "react";
import {
  FeaturedArticles,
  FeaturedArticlesSkeleton,
} from "@/features/home-page/components/featured-articles";

export const metadata: Metadata = {
  title: "خانه",
};

export default async function HomePage() {
  return (
    <Suspense fallback={<FeaturedArticlesSkeleton />}>
      <FeaturedArticles />
    </Suspense>
  );
}
