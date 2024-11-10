import {AxiosRequestConfig} from "axios";
import {apiClient} from "@/dal";

export async function fetchHomePageData(config?: AxiosRequestConfig) {
  const response = await apiClient.get("home", config);
  return response.data;
}
