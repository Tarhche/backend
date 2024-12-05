import {AxiosRequestConfig} from "axios";
import {privateDalDriver} from "./private-dal-driver";

export async function fetchUserProfile() {
  const response = await privateDalDriver.get("dashboard/profile");
  return response;
}

export async function fetchUserRoles(config?: AxiosRequestConfig) {
  const response = await privateDalDriver.get(
    "dashboard/profile/roles",
    config,
  );

  return response.data;
}

export async function updateUserProfile(data: any) {
  return await privateDalDriver.put("dashboard/profile", data);
}
