import {apiClient, apiPaths} from "@/dal";

export async function fetchAllArticlesByHashtag(hashtag: string) {
  const response = await apiClient.get(apiPaths.hashtags.show(hashtag));
  return response.data;
}
