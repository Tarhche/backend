import {apiClient} from "@/dal";

export async function fetchAllArticlesByHashtag(hashtag: string) {
  const response = await apiClient.get(`hashtags/${hashtag}`);
  return response.data;
}
