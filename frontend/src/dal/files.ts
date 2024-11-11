import {AxiosRequestConfig} from "axios";
import {apiClient} from "@/dal";

export async function fetchFiles(config?: AxiosRequestConfig) {
  const response = await apiClient.get("dashboard/files", config);
  return response.data;
}

export async function addNewFile(body: FormData) {
  return await apiClient.post("dashboard/files", body, {
    headers: {
      "Content-Type": "multipart/form-data",
    },
  });
}

export async function deleteFile(id: string) {
  return await apiClient.delete(`dashboard/files/${id}`);
}
