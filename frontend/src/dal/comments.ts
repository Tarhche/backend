import {AxiosRequestConfig} from "axios";
import {apiClient} from "@/dal";

export async function fetchUsersComments(config?: AxiosRequestConfig) {
  const response = await apiClient.get("dashboard/comments", config);
  return response.data;
}

export async function fetchArticleComments(articleUUID: string) {
  const article = await apiClient.get("comments", {
    params: {
      object_type: "article",
      object_uuid: articleUUID,
    },
  });
  return article.data;
}

export async function createArticleComment(body: {
  object_uuid: string;
  body: string;
  parent_uuid: string;
}) {
  const response = await apiClient.post("comments", {
    ...body,
    object_type: "article",
  });
  return response.data;
}
