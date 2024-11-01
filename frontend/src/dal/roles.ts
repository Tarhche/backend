import {apiClient} from "./api-client";

export async function fetchUserRolesByAccessToken() {
  const response = await apiClient.get("dashboard/roles");

  return response.data;
}
