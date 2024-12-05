"use server";
import {revalidatePath} from "next/cache";
import {redirect} from "next/navigation";
import {DALDriverError} from "@/dal/dal-driver-error";
import {createRole, updateRole} from "@/dal/private/roles";
import {APP_PATHS} from "@/lib/app-paths";

type FormState = {
  success: boolean;
  fieldErrors?: {
    name?: string;
    description?: string;
  };
};

export async function upsertRoleAction(
  formState: FormState,
  formData: FormData,
): Promise<FormState> {
  const roleId = formData.get("roleId")?.toString();

  try {
    const values: Record<string, string | string[] | null> = {
      name: formData.get("name")?.toString() || null,
      description: formData.get("description")?.toString() || null,
      permissions: formData.getAll("permissions").toString().split(",") || null,
      user_uuids: formData.get("user_uuids")?.toString().split(",") || null,
    };
    if (roleId === undefined) {
      await createRole(values);
    } else {
      values.uuid = formData.get("roleId")?.toString() || null;
      await updateRole(values);
    }
  } catch (error) {
    if (
      error instanceof DALDriverError &&
      (error.statusCode === 400 || error.statusCode === 422)
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

  revalidatePath(APP_PATHS.dashboard.roles.index);
  redirect(APP_PATHS.dashboard.roles.index);
}
