import {dalDriver} from "@/dal";

export async function fetchAllArticlesByHashtag(hashtag: string) {
  const response = await dalDriver.get(`hashtags/${hashtag}`);
  return response.data;
}
