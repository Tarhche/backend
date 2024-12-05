"use server";
import {revalidatePath} from "next/cache";
import {APP_PATHS} from "@/lib/app-paths";
import {deleteComment} from "@/dal/private/comments";

export async function deleteCommentAction(formData: FormData) {
  const commentId = formData.get("id")?.toString();
  if (commentId === undefined) {
    return;
  }
  await deleteComment(commentId);
  revalidatePath(APP_PATHS.dashboard.comments.index);
}
