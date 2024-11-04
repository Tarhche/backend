import {cookies} from "next/headers";
import jwt from "jsonwebtoken";

export function isUserLoggedIn() {
  const cookiesStore = cookies();
  const accessToken = cookiesStore.get("access_token")?.value;
  if (accessToken === undefined) {
    return false;
  }
  const decodedAccessToken = jwt.decode(accessToken, {
    json: true,
  });
  if (decodedAccessToken === null) {
    return null;
  }
  return true;
}
