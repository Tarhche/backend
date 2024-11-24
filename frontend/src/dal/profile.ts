import {apiClient, AxiosRequestConfig} from ".";

export async function fetchUserProfile() {
  const response = await apiClient.get("dashboard/profile");
  return response;
}

export async function fetchUserRoles(config?: AxiosRequestConfig) {
  const response = await apiClient.get("dashboard/profile/roles", config);
  return response.data;
}

export async function updateUserProfile(data: any) {
  return await apiClient.put("dashboard/profile", data);
}
