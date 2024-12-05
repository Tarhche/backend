"use server";
import {revalidatePath} from "next/cache";
import {DALDriverError} from "@/dal/dal-driver-error";
import {updateConfigs} from "@/dal/private/config";
import {convertFormDataActionToObject} from "@/lib/transformers";
import {APP_PATHS} from "@/lib/app-paths";

type FormState = {
  success: boolean;
  fieldErrors?: Record<string, string>;
};

export async function updateSettingAction(
  formState: FormState,
  formData: FormData,
): Promise<FormState> {
  try {
    const data = convertFormDataActionToObject(formData);
    if (typeof data.user_default_roles === "string") {
      data.user_default_roles = data.user_default_roles.split(",");
    }
    await updateConfigs(data);
  } catch (error) {
    if (error instanceof DALDriverError && error.statusCode === 400) {
      return {
        success: false,
        fieldErrors: error.response?.data.errors,
      };
    }
    return {
      success: false,
    };
  }

  revalidatePath(APP_PATHS.dashboard.settings);
  return {
    success: true,
  };
}
