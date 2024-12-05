"use server";
import {redirect} from "next/navigation";
import {revalidatePath} from "next/cache";
import {updateUserComment} from "@/dal/private/comments";
import {getRootUrl} from "@/lib/http";
import {APP_PATHS} from "@/lib/app-paths";

type FormState = {
  success?: boolean;
  errorMessage?: string;
};

export async function updateCommentAction(
  prevState: FormState,
  formData: FormData,
): Promise<FormState | never> {
  const message = formData.get("message")?.toString();
  const commentId = formData.get("id")?.toString();
  const approvalDate = formData.get("approvalDate")?.toString() || null;
  const objectId = formData.get("objectId")?.toString();
  const parentId = formData.get("parentId")?.toString() || null;

  try {
    await updateUserComment({
      body: message,
      uuid: commentId,
      approved_at: approvalDate,
      object_uuid: objectId,
      parent_uuid: parentId,
    });
  } catch {
    return {
      success: false,
      errorMessage: "ویرایش کامنت با خطا مواجه شد",
    };
  }

  revalidatePath(APP_PATHS.dashboard.comments.index);
  redirect(`${getRootUrl()}${APP_PATHS.dashboard.comments.index}`);
}
