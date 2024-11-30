import {AxiosRequestConfig} from "axios";
import {dalDriver} from "@/dal";

export async function fetchArticles(config?: AxiosRequestConfig) {
  const response = await dalDriver.get("articles", config);
  return response.data;
}

export async function fetchArticleByUUID(uuid: string) {
  const article = await dalDriver.get(`articles/${uuid}`);
  return article.data;
}

export async function fetchAllArticles(config?: AxiosRequestConfig) {
  const response = await dalDriver.get("dashboard/articles", config);
  return response.data;
}

export async function createArticle(data: any) {
  return await dalDriver.post("dashboard/articles", data);
}

export async function updateArticle(data: any) {
  return await dalDriver.put("dashboard/articles", data);
}

export async function fetchArticle(
  articleId: string,
  config?: AxiosRequestConfig,
) {
  const response = await dalDriver.get(
    `dashboard/articles/${articleId}`,
    config,
  );
  return response.data;
}
