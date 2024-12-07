import {publicDalDriver} from "./public-dal-driver";

export async function fetchArticleComments(articleUUID: string) {
  const article = await publicDalDriver.get("comments", {
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
  const response = await publicDalDriver.post("comments", {
    ...body,
    object_type: "article",
  });
  return response.data;
}
