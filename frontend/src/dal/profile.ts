import {apiClient, apiPaths} from ".";

export async function fetchUserProfile() {
  return await apiClient.get(apiPaths.dashbaord.profile);
}
