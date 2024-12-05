import {AxiosRequestConfig} from "axios";
import {privateDalDriver} from "./private-dal-driver";

export async function fetchUserBookmarks(config?: AxiosRequestConfig) {
  const response = await privateDalDriver.get("dashboard/my/bookmarks", config);
  return response.data;
}

export async function removeUserBookmark(id: string) {
  const response = await privateDalDriver.delete("dashboard/my/bookmarks", {
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
  try {
    const response = await privateDalDriver.post("bookmarks/exists", {
      object_type: "article",
      object_uuid: uuid,
    });

    return response.data?.exist;
  } catch {
    return undefined;
  }
}

export async function bookmarkArticle(body: {
  keep: boolean;
  uuid: string;
  title: string;
}) {
  const response = await privateDalDriver.put("bookmarks", {
    keep: body.keep,
    title: body.title,
    object_type: "article",
    object_uuid: body.uuid,
  });
  return response.data;
}
