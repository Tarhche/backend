"use server";
import {revalidatePath} from "next/cache";
import {redirect} from "next/navigation";
import {DALDriverError} from "@/dal/dal-driver-error";
import {updateProfilePassword} from "@/dal/private/password";
import {APP_PATHS} from "@/lib/app-paths";

type FormState = {
  success: boolean;
  fieldErrors?: {
    current_password?: string;
    new_password: string;
  };
};

export async function updateProfilePasswordAction(
  formState: FormState,
  formData: FormData,
): Promise<FormState> {
  const values: Record<string, string> = {};
  formData.forEach((value, key) => {
    if (key.includes("$") === false && value.toString()) {
      values[key] = value.toString();
    }
  });

  try {
    await updateProfilePassword(values);
  } catch (err) {
    if (err instanceof DALDriverError) {
      return {
        success: false,
        fieldErrors: err.response?.data.errors || {},
      };
    }
    return {
      success: false,
    };
  }

  revalidatePath(APP_PATHS.dashboard.profile.index);
  redirect(APP_PATHS.dashboard.profile.index);
}
