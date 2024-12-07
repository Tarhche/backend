import {type Metadata} from "next";
import {VerifyForm} from "@/features/auth/components/verify-form";

export const metadata: Metadata = {
  title: "تکمیل حساب کاربری",
};

type Props = {
  searchParams: {
    token: string;
  };
};

async function AccountVerificationPage(props: Props) {
  const token = props.searchParams.token;
  return <VerifyForm token={token} />;
}

export default AccountVerificationPage;
