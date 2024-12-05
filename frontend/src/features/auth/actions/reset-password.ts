"use server";
import {DALDriverError} from "@/dal/dal-driver-error";
import {resetPassword as changePassword} from "@/dal/public/auth";

type FormState = {
  success: boolean;
  fieldErrors?: {
    password: string;
  };
  errorMessage?: string[];
} | null;

export async function resetPassword(
  state: FormState,
  formData: FormData,
): Promise<FormState> {
  const newPassword = formData.get("new-password")?.toString();
  const confirmNewPassword = formData.get("confirm-new-password")?.toString();
  const token = formData.get("token")?.toString();

  if (newPassword !== confirmNewPassword) {
    return {
      success: false,
      fieldErrors: {
        password: "کلمه های عبور باید یکسان باشند.",
      },
    };
  }

  try {
    if (newPassword && token) {
      await changePassword(newPassword, token);
    } else {
      throw new Error();
    }
    return {
      success: true,
    };
  } catch (error) {
    if (error instanceof DALDriverError) {
      const errors = error.response?.data?.errors ?? {};
      if ("token" in errors) {
        return {
          success: false,
          errorMessage: [errors.token],
        };
      } else if (error.statusCode === 500) {
        return {
          success: false,
          errorMessage: ["خطایی سمت سرور اتفاق افتاد"],
        };
      } else {
        return {
          success: false,
          fieldErrors: errors,
        };
      }
    } else {
      return {
        success: false,
        errorMessage: ["خطایی ناشناخته اتفاق افتاد. لطفا مجددا تلاش نمایید"],
      };
    }
  }
}
