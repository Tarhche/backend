import {dalDriver} from "./driver/dal-driver";

export async function updateProfilePassword(data: any) {
  return await dalDriver.put("dashboard/password", data);
}
