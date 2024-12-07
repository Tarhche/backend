import {AxiosRequestConfig} from "axios";
import {privateDalDriver} from "./private-dal-driver";

export async function fetchRoles(config?: AxiosRequestConfig) {
  const response = await privateDalDriver.get("dashboard/roles", config);

  return response.data;
}

export async function fetchRole(roleId: string, config?: AxiosRequestConfig) {
  const response = await privateDalDriver.get(
    `dashboard/roles/${roleId}`,
    config,
  );

  return response.data;
}

export async function createRole(data: any) {
  return await privateDalDriver.post("dashboard/roles", data);
}

export async function updateRole(data: any) {
  return await privateDalDriver.put("dashboard/roles", data);
}

export async function deleteRole(roleId: string) {
  return await privateDalDriver.delete(`dashboard/roles/${roleId}`);
}
