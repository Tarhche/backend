"use server";
import {AxiosError} from "axios";
import {API} from "@/lib/api";

type FormState =
  | {
      success: boolean;
      fieldErrors?: {
        identity: string;
      };
      errorMessages?: string[];
    }
  | undefined;

export async function forgotPassword(
  prevState: FormState,
  formData: FormData,
): Promise<FormState> {
  const identity = formData.get("identity");
  try {
    await API.post("auth/password/forget", {
      identity,
    });
    return {
      success: true,
    };
  } catch (error) {
    if (error instanceof AxiosError) {
      return {
        success: false,
        fieldErrors: error.response?.data?.errors ?? {},
      };
    } else {
      return {
        success: false,
        errorMessages: ["خطایی ناشناخته رخ داد لطفا مجددا تلاش نمایید"],
      };
    }
  }
}
