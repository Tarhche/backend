import {AxiosRequestConfig} from "axios";
import {apiClient} from "./api-client";

export async function fetchUserRolesByAccessToken() {
  const response = await apiClient.get("dashboard/profile/roles");

  return response.data;
}

export async function fetchRoles(config?: AxiosRequestConfig) {
  const response = await apiClient.get("dashboard/roles", config);

  return response.data;
}

export async function deleteRole(roleId: string) {
  return await apiClient.delete(`dashboard/roles/${roleId}`);
}
