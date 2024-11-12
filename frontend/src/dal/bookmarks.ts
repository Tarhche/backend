import {AxiosRequestConfig} from "axios";
import {apiClient} from ".";

export async function fetchUserBookmarks(config?: AxiosRequestConfig) {
  const response = await apiClient.get("dashboard/my/bookmarks", config);
  return response.data;
}

export async function removeUserBookmark(id: string) {
  const response = await apiClient.delete("dashboard/my/bookmarks", {
    data: {
      object_type: "article",
      object_uuid: id,
    },
  });
  return response.data;
}

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
