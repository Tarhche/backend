"use server";
import {revalidatePath} from "next/cache";
import {deleteRole} from "@/dal";
import {APP_PATHS} from "@/lib/app-paths";

export async function deleteRoleAction(formData: FormData) {
  const fileId = formData.get("id")?.toString();
  if (fileId === undefined) {
    return;
  }
  await deleteRole(fileId);
  revalidatePath(APP_PATHS.dashboard.files);
}
