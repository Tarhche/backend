import {AxiosRequestConfig} from "axios";
import {apiClient, apiPaths} from "@/dal";

export async function fetchArticles(config?: AxiosRequestConfig) {
  const response = await apiClient.get(apiPaths.articles.list, config);
  return response.data;
}

export async function fetchArticleByUUID(uuid: string) {
  const article = await apiClient.get(apiPaths.articles.show(uuid));
  return article.data;
}
