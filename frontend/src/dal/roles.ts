import {AxiosRequestConfig} from "axios";
import {dalDriver} from "./driver/dal-driver";

export async function fetchRoles(config?: AxiosRequestConfig) {
  const response = await dalDriver.get("dashboard/roles", config);

  return response.data;
}

export async function fetchRole(roleId: string, config?: AxiosRequestConfig) {
  const response = await dalDriver.get(`dashboard/roles/${roleId}`, config);

  return response.data;
}

export async function createRole(data: any) {
  return await dalDriver.post("dashboard/roles", data);
}

export async function updateRole(data: any) {
  return await dalDriver.put("dashboard/roles", data);
}

export async function deleteRole(roleId: string) {
  return await dalDriver.delete(`dashboard/roles/${roleId}`);
}
