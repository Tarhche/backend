"use server";
import {revalidatePath} from "next/cache";
import {APP_PATHS} from "@/lib/app-paths";
import {removeUserBookmark} from "@/dal/private/bookmarks";

export async function removeBookmarkAction(formData: FormData) {
  const fileId = formData.get("id")?.toString();
  if (fileId === undefined) {
    return;
  }
  await removeUserBookmark(fileId);
  revalidatePath(APP_PATHS.dashboard.files);
}
