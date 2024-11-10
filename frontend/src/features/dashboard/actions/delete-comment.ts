"use server";
import {revalidatePath} from "next/cache";
import {APP_PATHS} from "@/lib/app-paths";
import {apiClient} from "@/dal";

export async function deleteComment(formData: FormData) {
  const commentId = formData.get("id")?.toString();
  if (commentId === undefined) {
    return;
  }
  await apiClient.delete(`/dashboard/comments/${commentId}`);
  revalidatePath(APP_PATHS.dashboard.comments.index);
}
