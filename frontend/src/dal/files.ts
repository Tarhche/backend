import {AxiosRequestConfig} from "axios";
import {dalDriver} from "@/dal";

export async function fetchFiles(config?: AxiosRequestConfig) {
  const response = await dalDriver.get("dashboard/files", config);
  return response.data;
}

export async function addNewFile(body: FormData) {
  return await dalDriver.post("dashboard/files", body, {
    headers: {
      "Content-Type": "multipart/form-data",
    },
  });
}

export async function deleteFile(id: string) {
  return await dalDriver.delete(`dashboard/files/${id}`);
}
