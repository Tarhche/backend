"use server";
import * as z from "zod";
import {DALDriverError} from "@/dal/dal-driver-error";
import {verifyUser as completeUserProfile} from "@/dal/public/auth";

const FIELDS_SCHEMA = z
  .object({
    name: z
      .string({
        required_error: "نام الزامی است",
      })
      .min(3, "نام باید حداقل دارای ۳ کاراکتر باشد"),
    username: z
      .string({
        required_error: "نام کاربری الزامی است",
      })
      .min(3, "نام کاربری باید حداقل دارای ۳ کاراکتر باشد"),
    password: z
      .string({
        required_error: "کلمه عبور الزامی است",
      })
      .min(6, "کلمه عبور باید حداقل دارای ۶ کاراکتر باشد"),
    repassword: z
      .string({
        required_error: "تکرار کلمه عبور الزامی است",
      })
      .min(6, "کلمه عبور باید حداقل دارای ۶ کاراکتر باشد"),
  })
  .superRefine(({password, repassword}, ctx) => {
    if (password !== repassword) {
      const common: z.IssueData = {
        code: "custom",
        message: "کلمه های عبور با هم مطابقت ندارند",
      };
      ctx.addIssue({
        ...common,
        path: ["repassword"],
      });
      ctx.addIssue({
        ...common,
        path: ["password"],
      });
    }
  });

type Schema = z.infer<typeof FIELDS_SCHEMA>;

type FormKeys = keyof Schema;

type FormState = {
  success: boolean;
  fieldErrors?: {
    [P in FormKeys]?: string[] | undefined;
  };
  nonFieldErrors?: string[];
};

export async function verifyUser(
  state: FormState,
  formData: FormData,
): Promise<FormState> {
  const data: Record<string, string> = {};
  formData.forEach((value, key) => {
    if (value instanceof File === false) {
      data[key] = value as string;
    }
  });
  const fieldsValidation = await FIELDS_SCHEMA.safeParseAsync(data);
  const nonFieldErrors: string[] = [];
  if (Boolean(data.token) === false) {
    nonFieldErrors.push(
      "توکن ثبت نامی یافت نشد. لطفا مطمئن شوید از طریق ایمیلتان به این صفحه راه یافته اید",
    );
  }
  if (fieldsValidation.success === false || nonFieldErrors.length >= 1) {
    return {
      success: false,
      fieldErrors: fieldsValidation.error?.flatten().fieldErrors,
      nonFieldErrors: nonFieldErrors,
    };
  } else {
    try {
      await completeUserProfile(data);
      return {
        success: true,
      };
    } catch (e) {
      if (e instanceof DALDriverError) {
        const errors = e.response?.data.errors;
        if (errors && errors.token) {
          nonFieldErrors.push("توکن ثبت نام معبتر نیست");
        }
        if (errors && errors.identity) {
          nonFieldErrors.push("این حساب در حال حاضر موجود است");
        }
        return {
          success: false,
          nonFieldErrors: nonFieldErrors,
        };
      }
      return {
        success: false,
        nonFieldErrors: ["عملیات با خطا مواجه شد لطفا دوباره تلاش نمایید"],
      };
    }
  }
}
