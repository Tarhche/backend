import {AxiosRequestConfig} from "axios";
import {publicDalDriver} from "./public-dal-driver";

export async function fetchHomePageData(config?: AxiosRequestConfig) {
  const response = await publicDalDriver.get("home", config);
  return response.data;
}
