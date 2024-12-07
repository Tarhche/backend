"use server";
import {DALDriverError} from "@/dal/dal-driver-error";
import {registerUser as signUpUser} from "@/dal/public/auth";

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
  const email = formData.get("email")?.toString();
  try {
    if (email === undefined) {
      throw new Error();
    }
    await signUpUser(email);
    return {
      success: true,
      message: "",
    };
  } catch (e) {
    if (e instanceof DALDriverError) {
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
