"use server";
import {revalidatePath} from "next/cache";
import {redirect} from "next/navigation";
import {updatePassword} from "@/dal/private/users";
import {APP_PATHS} from "@/lib/app-paths";

type FormStatus = {
  success: boolean;
  fieldErrors?: {
    password?: string;
    rePassword?: string;
  };
};

export async function updateUserPasswordAction(
  formState: FormStatus,
  formData: FormData,
): Promise<FormStatus> {
  const userId = formData.get("userId")?.toString() ?? "";
  const password = formData.get("password")?.toString() ?? "";
  const repassword = formData.get("repassword")?.toString() ?? "";
  if (password !== repassword) {
    return {
      success: false,
      fieldErrors: {
        password: "کلمه های عبور مطابقط ندارند",
        rePassword: "کلمه های عبور مطابقط ندارند",
      },
    };
  }
  try {
    await updatePassword({
      new_password: password,
      uuid: userId,
    });
  } catch {
    return {
      success: false,
    };
  }
  revalidatePath(APP_PATHS.dashboard.users.edit(userId));
  redirect(APP_PATHS.dashboard.users.edit(userId));
}
