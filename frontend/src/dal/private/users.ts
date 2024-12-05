import {AxiosRequestConfig} from "axios";
import {privateDalDriver} from "./private-dal-driver";

export async function fetchUsers(config?: AxiosRequestConfig) {
  const response = await privateDalDriver.get("dashboard/users", config);
  return response.data;
}

export async function fetchUser(userId: string, config?: AxiosRequestConfig) {
  const response = await privateDalDriver.get(
    `dashboard/users/${userId}`,
    config,
  );
  return response.data;
}

export async function createUser(data: Record<string, string>) {
  return await privateDalDriver.post("dashboard/users", data);
}

export async function updateUser(data: Record<string, string>) {
  return await privateDalDriver.put("dashboard/users", data);
}

export async function deleteUser(id: string) {
  return await privateDalDriver.delete(`dashboard/users/${id}`);
}

export async function updatePassword(data: Record<string, string>) {
  return await privateDalDriver.put(`dashboard/users/password`, data);
}
