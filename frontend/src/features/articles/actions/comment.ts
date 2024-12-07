"use server";
import {createArticleComment} from "@/dal/public/comments";

type FormState = {
  success?: boolean;
  errorMessage?: string;
};

export async function comment(
  state: FormState,
  formData: FormData,
): Promise<FormState> {
  const objectUUID = formData.get("object-uuid")?.toString() ?? "";
  const parentUUID = formData.get("parent-uuid")?.toString() ?? "";
  const body = formData.get("body")?.toString() ?? "";
  if (body.trim().length <= 5) {
    return {
      success: false,
      errorMessage: "متن دیدگاه شما کوتاه است",
    };
  }

  try {
    await createArticleComment({
      object_uuid: objectUUID,
      parent_uuid: parentUUID,
      body: body,
    });
    return {
      success: true,
    };
  } catch {
    return {
      success: false,
    };
  }
}
