"use server";
import {revalidatePath} from "next/cache";
import {DALDriverError} from "@/dal/dal-driver-error";
import {updateUserProfile} from "@/dal/private/profile";
import {APP_PATHS} from "@/lib/app-paths";

type FormState = {
  success: boolean | null;
  fieldErrors?: {
    name?: string;
    email?: string;
    username?: string;
  };
};

export async function updateProfileAction(
  formState: FormState,
  formData: FormData,
): Promise<FormState> {
  const values: Record<string, string> = {};
  formData.forEach((value, key) => {
    if (key.includes("$") === false && Boolean(value)) {
      values[key] = value.toString() || "";
    }
  });

  try {
    await updateUserProfile(values);
  } catch (err) {
    if (err instanceof DALDriverError && err.statusCode === 400) {
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
  return {
    success: true,
  };
}
