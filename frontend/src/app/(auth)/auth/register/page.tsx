import {type Metadata} from "next";
import {RegisterForm} from "@/features/auth/components/register-form";

export const metadata: Metadata = {
  title: "ثبت نام",
};

function RegisterPage() {
  return <RegisterForm />;
}

export default RegisterPage;
