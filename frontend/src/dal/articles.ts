import {apiClient, apiPaths} from "@/dal";
import {waitFor} from "@/lib/sleep";

export async function fetchArticleByUUID(uuid: string) {
  const article = await apiClient.get(apiPaths.articles.show(uuid));
  return article.data;
}
