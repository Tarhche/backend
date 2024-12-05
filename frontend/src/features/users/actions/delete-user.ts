"use server";
import {revalidatePath} from "next/cache";
import {APP_PATHS} from "@/lib/app-paths";
import {deleteUser} from "@/dal/private/users";

export async function deleteUserAction(formData: FormData) {
  const userID = formData.get("id")?.toString();
  if (userID === undefined) {
    return;
  }
  await deleteUser(userID);
  revalidatePath(APP_PATHS.dashboard.users.index);
}
