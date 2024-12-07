import {publicDalDriver} from "./public-dal-driver";

export async function fetchAllArticlesByHashtag(hashtag: string) {
  const response = await publicDalDriver.get(`hashtags/${hashtag}`);
  return response.data;
}
