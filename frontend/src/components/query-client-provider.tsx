"use client";
import {ReactNode} from "react";
import {
  QueryClient,
  QueryClientProvider as QueryClientProvider_,
} from "@tanstack/react-query";
import {fetchWrapper} from "@/lib/client-fetch-wrapper";

type Props = {
  children: ReactNode;
};

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      queryFn: async (config) => {
        return await fetchWrapper(`/api/${config.queryKey[0]}`);
      },
    },
  },
});

export function QueryClientProvider({children}: Props) {
  return (
    <QueryClientProvider_ client={queryClient}>{children}</QueryClientProvider_>
  );
}
