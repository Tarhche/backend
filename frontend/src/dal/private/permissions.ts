import {privateDalDriver} from "./private-dal-driver";

export async function fetchAllPermissions() {
  const response = await privateDalDriver.get("dashboard/permissions");
  return response.data;
}
