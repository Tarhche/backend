import {apiClient} from "@/dal";

export async function fetchConfigs() {
  const response = await apiClient.get("dashboard/config");
  return response.data;
}

export async function updateConfigs(data: any) {
  return await apiClient.put("dashboard/config", data);
}
