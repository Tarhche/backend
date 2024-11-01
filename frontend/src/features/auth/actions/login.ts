"use server";
import {redirect} from "next/navigation";
import {cookies} from "next/headers";
import {loginUser} from "@/dal/auth";
import {ACCESS_TOKEN_EXP, REFRESH_TOKEN_EXP} from "@/constants/numbers";

type FormState = {
  success: boolean;
  fieldErrors?: {
    identity?: string;
    password?: string;
  };
  errorMessages?: string[];
} | null;

export async function login(
  prevState: FormState,
  formData: FormData,
): Promise<FormState> {
  const identity = formData.get("identity")?.toString() ?? "";
  const password = formData.get("password")?.toString() ?? "";
  const shouldPersistUser =
    formData.get("remember")?.toString() === "on" ? true : false;
  const callbackUrl = formData.get("callbackUrl")?.toString();
  const isDataValid =
    typeof identity === "string" &&
    typeof password === "string" &&
    typeof shouldPersistUser === "boolean";
  if (isDataValid) {
    try {
      const response = await loginUser(identity, password);
      cookies().set("access_token", response.access_token, {
        maxAge: ACCESS_TOKEN_EXP,
        httpOnly: true,
        secure: true,
      });
      cookies().set("refresh_token", response.refresh_token, {
        maxAge: REFRESH_TOKEN_EXP,
        httpOnly: true,
        secure: true,
      });
    } catch {
      return {
        success: false,
        errorMessages: [
          " ایمیل یا نام کاربری یا کلمه عبورتان را اشتباه وارد کرده اید",
        ],
      };
    }
    if (callbackUrl) {
      redirect(callbackUrl);
    }
    redirect("/dashboard");
  }
  return {
    success: false,
  };
}
