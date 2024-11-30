"use server";
import {revalidatePath} from "next/cache";
import {APP_PATHS} from "@/lib/app-paths";
import {dalDriver} from "@/dal";

export async function deleteArticle(formData: FormData) {
  const articleId = formData.get("id")?.toString();
  if (articleId === undefined) {
    return;
  }

  await dalDriver.delete(`/dashboard/articles/${articleId}`);
  revalidatePath(APP_PATHS.dashboard.articles.index);
}
