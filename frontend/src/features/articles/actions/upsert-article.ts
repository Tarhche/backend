"use server";
import {revalidatePath} from "next/cache";
import {redirect} from "next/navigation";
import {DALDriverError} from "@/dal/dal-driver-error";
import {createArticle, updateArticle} from "@/dal/private/articles";
import {APP_PATHS} from "@/lib/app-paths";

type FormState = {
  success: boolean;
  fieldErrors?: {
    title?: string;
    excerpt?: string;
    body?: string;
  };
};

export async function upsertArticleAction(
  formState: FormState,
  formData: FormData,
): Promise<FormState> {
  const values: Record<string, string | string[]> = {};
  formData.forEach((v, k) => {
    if (v) {
      values[k] = v.toString();
    }
  });

  values.tags = formData.get("tags")?.toString().split(",") || "";
  const articleId = formData.get("uuid")?.toString();

  try {
    if (articleId) {
      await updateArticle(values);
    } else {
      await createArticle(values);
    }
  } catch (err) {
    if (
      err instanceof DALDriverError &&
      (err.statusCode === 400 || err.statusCode == 401)
    ) {
      return {
        success: false,
        fieldErrors: err.response?.data.errors ?? {},
      };
    }
    return {
      success: false,
    };
  }

  revalidatePath(APP_PATHS.dashboard.articles.index);
  if (articleId) {
    revalidatePath(APP_PATHS.dashboard.articles.edit(articleId));
  }
  redirect(APP_PATHS.dashboard.articles.index);
}
