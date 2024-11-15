import {apiClient} from "./api-client";

export async function fetchAllPermissions() {
  const response = await apiClient.get("dashboard/permissions");
  return response.data;
}
