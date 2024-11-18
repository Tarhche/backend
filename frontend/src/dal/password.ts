import {apiClient} from "./api-client";

export async function updateProfilePassword(data: any) {
  return await apiClient.put("dashboard/password", data);
}
