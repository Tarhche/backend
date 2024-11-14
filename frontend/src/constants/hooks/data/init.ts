import {useQuery} from "@tanstack/react-query";
import {AuthState} from "@/types/api-responses/init";

export function useInit() {
  return useQuery<AuthState>({
    queryKey: ["init"],
    retry: 1,
  });
}
