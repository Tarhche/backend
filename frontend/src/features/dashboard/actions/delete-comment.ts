"use server";
import {revalidatePath} from "next/cache";
import {APP_PATHS} from "@/lib/app-paths";
import {apiClient, apiPaths} from "@/dal";

export async function deleteComment(formData: FormData) {
  const commentId = formData.get("id")?.toString();
  if (commentId === undefined) {
    return;
  }
  await apiClient.delete(apiPaths.dashbaord.usersCommentsDetail(commentId));
  revalidatePath(APP_PATHS.dashboard.comments.index);
}
