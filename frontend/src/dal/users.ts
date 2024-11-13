import {AxiosRequestConfig} from "axios";
import {apiClient} from "./api-client";

export async function fetchUsers(config?: AxiosRequestConfig) {
  const response = await apiClient.get("dashboard/users", config);
  return response.data;
}

export async function deleteUser(id: string) {
  return await apiClient.delete(`dashboard/users/${id}`);
}
