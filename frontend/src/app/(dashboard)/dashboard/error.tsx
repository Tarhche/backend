"use client";
import {useEffect} from "react";
import {Error} from "@/components/error";

type Props = {
  error: Error & {digest: string};
  reset: () => void;
};

function ErrorPage({error, reset}: Props) {
  useEffect(() => {
    console.error(error.digest);
  }, [error]);

  return <Error onReset={reset} />;
}

export default ErrorPage;
