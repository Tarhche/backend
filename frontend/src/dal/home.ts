import {AxiosRequestConfig} from "axios";
import {dalDriver} from "@/dal";

export async function fetchHomePageData(config?: AxiosRequestConfig) {
  const response = await dalDriver.get("home", config);
  return response.data;
}
