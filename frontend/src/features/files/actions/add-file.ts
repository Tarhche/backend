"use server";
import {revalidatePath} from "next/cache";
import {APP_PATHS} from "@/lib/app-paths";
import {addNewFile} from "@/dal/private/files";

export async function addFileAction(formData: FormData): Promise<any> {
  await addNewFile(formData);
  revalidatePath(APP_PATHS.dashboard.articles.index);
}
