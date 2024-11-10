import {AxiosRequestConfig} from "axios";
import {apiClient} from "@/dal";

export async function fetchArticles(config?: AxiosRequestConfig) {
  const response = await apiClient.get("articles", config);
  return response.data;
}

export async function fetchArticleByUUID(uuid: string) {
  const article = await apiClient.get(`articles/${uuid}`);
  return article.data;
}
