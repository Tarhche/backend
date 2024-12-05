import {privateDalDriver} from "./private-dal-driver";

export async function updateProfilePassword(data: any) {
  return await privateDalDriver.put("dashboard/password", data);
}
