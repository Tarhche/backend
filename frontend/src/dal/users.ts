import {AxiosRequestConfig} from "axios";
import {dalDriver} from "./driver/dal-driver";

export async function fetchUsers(config?: AxiosRequestConfig) {
  const response = await dalDriver.get("dashboard/users", config);
  return response.data;
}

export async function fetchUser(userId: string, config?: AxiosRequestConfig) {
  const response = await dalDriver.get(`dashboard/users/${userId}`, config);
  return response.data;
}

export async function createUser(data: Record<string, string>) {
  return await dalDriver.post("dashboard/users", data);
}

export async function updateUser(data: Record<string, string>) {
  return await dalDriver.put("dashboard/users", data);
}

export async function deleteUser(id: string) {
  return await dalDriver.delete(`dashboard/users/${id}`);
}

export async function updatePassword(data: Record<string, string>) {
  return await dalDriver.put(`dashboard/users/password`, data);
}
