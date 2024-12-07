"use server";
import {revalidatePath} from "next/cache";
import {APP_PATHS} from "@/lib/app-paths";
import {deleteFile} from "@/dal/private/files";

export async function deleteFileAction(formData: FormData) {
  const fileId = formData.get("id")?.toString();
  if (fileId === undefined) {
    return;
  }
  await deleteFile(fileId);
  revalidatePath(APP_PATHS.dashboard.files);
}
