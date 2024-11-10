"use server";
import {revalidatePath} from "next/cache";
import {APP_PATHS} from "@/lib/app-paths";
import {apiClient} from "@/dal";

export async function deleteArticle(formData: FormData) {
  const articleId = formData.get("id")?.toString();
  if (articleId === undefined) {
    return;
  }
  await apiClient.delete(`/dashboard/articles/${articleId}`);
  revalidatePath(APP_PATHS.dashboard.articles.index);
}
