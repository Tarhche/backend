import {AxiosRequestConfig} from "axios";
import {privateDalDriver} from "./private-dal-driver";

export async function fetchAllComments(config?: AxiosRequestConfig) {
  const response = await privateDalDriver.get("dashboard/comments", config);
  return response.data;
}

export async function fetchUserComments(config?: AxiosRequestConfig) {
  const response = await privateDalDriver.get("dashboard/my/comments", config);
  return response.data;
}

export async function fetchUsersDetailComments(
  id: string,
  config?: AxiosRequestConfig,
) {
  const response = await privateDalDriver.get(
    `dashboard/comments/${id}`,
    config,
  );
  return response.data;
}

export async function updateUserComment(body: any) {
  const response = await privateDalDriver.put(`dashboard/comments`, {
    object_type: "article",
    ...body,
  });
  return response.data;
}

export async function deleteComment(commentId: string) {
  return await privateDalDriver.delete(`/dashboard/comments/${commentId}`);
}
