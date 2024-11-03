import {apiClient, apiPaths} from "@/dal";

export async function fetchArticleComments(articleUUID: string) {
  const article = await apiClient.get(apiPaths.comments.list, {
    params: {
      object_type: "article",
      object_uuid: articleUUID,
    },
  });
  return article.data;
}
