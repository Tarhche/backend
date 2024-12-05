"use server";
import {revalidatePath} from "next/cache";
import {APP_PATHS} from "@/lib/app-paths";
import {privateDalDriver} from "@/dal/private/private-dal-driver";

export async function deleteArticle(formData: FormData) {
  const articleId = formData.get("id")?.toString();
  if (articleId === undefined) {
    return;
  }

  await privateDalDriver.delete(`/dashboard/articles/${articleId}`);
  revalidatePath(APP_PATHS.dashboard.articles.index);
}
