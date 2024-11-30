import {dalDriver} from "@/dal";

export async function fetchConfigs() {
  const response = await dalDriver.get("dashboard/config");
  return response.data;
}

export async function updateConfigs(data: any) {
  return await dalDriver.put("dashboard/config", data);
}
