"use server";
import {apiClient, apiPaths} from "@/dal";
import {AxiosError} from "axios";

type SuccessRegisterState = {
  success: true;
  message: string;
};

type FailureRegisterState = {
  success: false;
  errorMessage: string;
};

type UntouchedState = {
  success: undefined;
};

type State = SuccessRegisterState | FailureRegisterState | UntouchedState;

export async function registerUser(
  state: State,
  formData: FormData,
): Promise<State> {
  const email = formData.get("email");
  try {
    await apiClient.post(apiPaths.auth.register, {
      identity: email,
    });
    return {
      success: true,
      message: "",
    };
  } catch (e) {
    if (e instanceof AxiosError) {
      const errors = e.response?.data.errors;
      if (Boolean(errors?.identity)) {
        return {
          success: false,
          errorMessage:
            "ایمیلی که وارد کرده اید از قبل موجود است و نمی توانید از آن استفاده کنید",
        };
      }
    }
    return {
      success: false,
      errorMessage: "خطایی ناشناخته اتفاق افتاد لطفا دوباره تلاش نمایید",
    };
  }
}
