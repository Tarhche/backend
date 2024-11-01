import {LoginForm} from "@/features/auth/components/login-form";

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
