import {apiClient} from ".";

export async function fetchUserProfile() {
  const response = await apiClient.get("dashboard/profile");
  return response;
}

export async function updateUserProfile(data: any) {
  return await apiClient.put("dashboard/profile", data);
}
