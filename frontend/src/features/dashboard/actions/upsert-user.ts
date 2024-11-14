"use server";
import {revalidatePath} from "next/cache";
import {redirect} from "next/navigation";
import {createUser, updateUser, AxiosError} from "@/dal";
import {APP_PATHS} from "@/lib/app-paths";

type FormState = {
  success: boolean;
  fieldErrors?: {
    email: string;
    name: string;
    username: string;
    password: string;
    avatar: string;
  };
};

export async function upsertUserAction(
  formState: FormState,
  formData: FormData,
): Promise<FormState> {
  const values: Record<string, string> = {};
  formData.forEach((value, key) => {
    if (key.includes("$") === false && Boolean(value)) {
      values[key] = value.toString();
    }
  });
  try {
    if (values.uuid === undefined) {
      await createUser(values);
    } else {
      await updateUser(values);
    }
  } catch (error) {
    if (
      error instanceof AxiosError &&
      (error.status === 400 || error.status === 422)
    ) {
      return {
        success: false,
        fieldErrors: error.response?.data.errors ?? {},
      };
    }
    return {
      success: false,
    };
  }
  revalidatePath(APP_PATHS.dashboard.users.index);
  redirect(APP_PATHS.dashboard.users.index);
}
