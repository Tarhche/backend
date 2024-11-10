import {apiClient} from ".";

export async function checkBookmarkStatus(
  uuid?: string,
): Promise<boolean | undefined> {
  if (uuid === undefined) {
    return undefined;
  }
  const response = await apiClient.post("bookmarks/exists", {
    object_type: "article",
    object_uuid: uuid,
  });

  return response.data?.exist;
}

export async function bookmarkArticle(body: {
  keep: boolean;
  uuid: string;
  title: string;
}) {
  const response = await apiClient.put("bookmarks", {
    keep: body.keep,
    title: body.title,
    object_type: "article",
    object_uuid: body.uuid,
  });
  return response.data;
}
