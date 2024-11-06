import {type Metadata} from "next";
import {LoginForm} from "@/features/auth/components/login-form";

export const metadata: Metadata = {
  title: "ورود",
};

type Props = {
  searchParams: {
    callbackUrl?: string;
  };
};

function LoginPage({searchParams}: Props) {
  const callbackUrl = searchParams.callbackUrl;
  return <LoginForm callbackUrl={callbackUrl} />;
}

export default LoginPage;
