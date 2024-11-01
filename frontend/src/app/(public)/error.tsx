"use client";
import {Error} from "@/components/error";

function ErrorPage({reset}: {reset: () => void}) {
  return <Error onReset={reset} />;
}

export default ErrorPage;
