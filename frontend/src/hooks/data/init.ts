import {useQuery} from "@tanstack/react-query";
import {AuthState} from "@/types/api-responses/init";
// import {waitFor} from "@/lib/sleep";

export function useInit() {
  return useQuery<AuthState>({
    queryKey: ["init"],
    retry: 1,
  });
}
