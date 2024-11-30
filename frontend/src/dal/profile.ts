import {dalDriver, AxiosRequestConfig} from ".";

export async function fetchUserProfile() {
  const response = await dalDriver.get("dashboard/profile");
  return response;
}

export async function fetchUserRoles(config?: AxiosRequestConfig) {
  const response = await dalDriver.get("dashboard/profile/roles", config);
  return response.data;
}

export async function updateUserProfile(data: any) {
  return await dalDriver.put("dashboard/profile", data);
}
