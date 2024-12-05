"use server";
import {DALDriverError} from "@/dal/dal-driver-error";
import {forgotPassword as recoverPassword} from "@/dal/public/auth";

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
  const identity = formData.get("identity")?.toString();
  try {
    if (identity === undefined) {
      throw new Error();
    }
    await recoverPassword(identity);
    return {
      success: true,
    };
  } catch (error) {
    if (error instanceof DALDriverError) {
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
