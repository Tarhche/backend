import {dalDriver} from "./driver/dal-driver";

export async function fetchAllPermissions() {
  const response = await dalDriver.get("dashboard/permissions");
  return response.data;
}
