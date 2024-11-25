"use client";
import {Error} from "@/components/error";
import {PERMISSION_DENIED} from "@/constants/strings";
import {PermissionDeniedError} from "@/components/errors/dashboard-permission-denied";

type Props = {
  error: Error;
  reset: () => void;
};

function ErrorPage({error, reset}: Props) {
  if (error.message === PERMISSION_DENIED) {
    return <PermissionDeniedError />;
  }

  return <Error onReset={reset} />;
}

export default ErrorPage;
