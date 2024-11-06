import {type Metadata} from "next";
import {ForgotPasswordForm} from "@/features/auth/components/forgot-password-form";

export const metadata: Metadata = {
  title: "فراموشی کلمه عبور",
};

function RegisterPage() {
  return <ForgotPasswordForm />;
}

export default RegisterPage;
