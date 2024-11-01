"use server";
import {AxiosError} from "axios";
import {apiClient, apiPaths} from "@/dal";

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
  const newPassword = formData.get("new-password");
  const confirmNewPassword = formData.get("confirm-new-password");
  const token = formData.get("token");

  if (newPassword !== confirmNewPassword) {
    return {
      success: false,
      fieldErrors: {
        password: "کلمه های عبور باید یکسان باشند.",
      },
    };
  }

  try {
    await apiClient.post(apiPaths.auth.resetPassword, {
      password: newPassword,
      token,
    });
    return {
      success: true,
    };
  } catch (error) {
    if (error instanceof AxiosError) {
      const errors = error.response?.data?.errors ?? {};
      if ("token" in errors) {
        return {
          success: false,
          errorMessage: [errors.token],
        };
      } else if (error.status === 500) {
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
