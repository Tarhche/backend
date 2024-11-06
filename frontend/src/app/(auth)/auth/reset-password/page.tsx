import {type Metadata} from "next";
import {ResetPasswordForm} from "@/features/auth/components/reset-password-form";

export const metadata: Metadata = {
  title: "بازیابی کلمه عبور",
};

type Props = {
  searchParams: {
    token: string;
  };
};

async function ResetPasswordPage(props: Props) {
  const token = props.searchParams.token;
  return <ResetPasswordForm token={token} />;
}

export default ResetPasswordPage;
