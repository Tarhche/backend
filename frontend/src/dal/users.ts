import {AxiosRequestConfig} from "axios";
import {apiClient} from "./api-client";

export async function fetchUsers(config?: AxiosRequestConfig) {
  const response = await apiClient.get("dashboard/users", config);
  return response.data;
}

export async function fetchUser(userId: string, config?: AxiosRequestConfig) {
  const response = await apiClient.get(`dashboard/users/${userId}`, config);
  return response.data;
}

export async function createUser(data: Record<string, string>) {
  return await apiClient.post("dashboard/users", data);
}

export async function updateUser(data: Record<string, string>) {
  return await apiClient.put("dashboard/users", data);
}

export async function deleteUser(id: string) {
  return await apiClient.delete(`dashboard/users/${id}`);
}

export async function updatePassword(data: Record<string, string>) {
  return await apiClient.put(`dashboard/users/password`, data);
}
