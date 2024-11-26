"use client";
import {Error} from "@/components/error";
import {PermissionDeniedError} from "@/components/errors/dashboard-permission-denied";
import {APIClientUnauthorizedError} from "@/dal/api-client-errors";

type Props = {
  error: Error;
  reset: () => void;
};

function ErrorPage({error, reset}: Props) {
  if (error instanceof APIClientUnauthorizedError) {
    return <PermissionDeniedError />;
  }

  return <Error onReset={reset} />;
}

export default ErrorPage;
