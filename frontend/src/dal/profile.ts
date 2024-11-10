import {apiClient} from ".";

export async function fetchUserProfile() {
  const response = await apiClient.get("dashboard/profile");
  return response;
}
