import {privateDalDriver} from "./private-dal-driver";

export async function fetchConfigs() {
  const response = await privateDalDriver.get("dashboard/config");
  return response.data;
}

export async function updateConfigs(data: any) {
  return await privateDalDriver.put("dashboard/config", data);
}
