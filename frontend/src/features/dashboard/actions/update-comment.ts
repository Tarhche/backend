"use server";
import {redirect} from "next/navigation";
import {revalidatePath} from "next/cache";
import {updateUserComment} from "@/dal";
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
  const message = formData.get("message");
  const commentId = formData.get("id");
  const approvalDate = formData.get("approvalDate");
  const objectId = formData.get("objectId");
  try {
    await updateUserComment({
      body: message,
      uuid: commentId,
      approved_at: approvalDate,
      object_uuid: objectId,
    });
  } catch (err) {
    return {
      success: false,
      errorMessage: "ویرایش کامنت با خطا مواجه شد",
    };
  }
  revalidatePath(APP_PATHS.dashboard.comments.index);
  redirect(`${getRootUrl()}${APP_PATHS.dashboard.comments.index}`);
}
