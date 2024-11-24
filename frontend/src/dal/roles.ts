import {AxiosRequestConfig} from "axios";
import {apiClient} from "./api-client";

export async function fetchRoles(config?: AxiosRequestConfig) {
  const response = await apiClient.get("dashboard/roles", config);

  return response.data;
}

export async function fetchRole(roleId: string, config?: AxiosRequestConfig) {
  const response = await apiClient.get(`dashboard/roles/${roleId}`, config);

  return response.data;
}

export async function createRole(data: any) {
  return await apiClient.post("dashboard/roles", data);
}

export async function updateRole(data: any) {
  return await apiClient.put("dashboard/roles", data);
}

export async function deleteRole(roleId: string) {
  return await apiClient.delete(`dashboard/roles/${roleId}`);
}
