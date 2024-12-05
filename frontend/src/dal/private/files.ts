import {AxiosRequestConfig} from "axios";
import {privateDalDriver} from "./private-dal-driver";

export async function fetchFiles(config?: AxiosRequestConfig) {
  const response = await privateDalDriver.get("dashboard/files", config);
  return response.data;
}

export async function addNewFile(body: FormData) {
  return await privateDalDriver.post("dashboard/files", body, {
    headers: {
      "Content-Type": "multipart/form-data",
    },
  });
}

export async function deleteFile(id: string) {
  return await privateDalDriver.delete(`dashboard/files/${id}`);
}
