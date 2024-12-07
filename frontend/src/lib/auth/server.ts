import jwt from "jsonwebtoken";
import {getCredentialsFromCookies} from "../http";

export function decodeJWT(token: string) {
  return jwt.decode(token ?? "", {
    json: true,
  });
}

/**
  This function retrieves the access or refresh token from cookies and verifies its validity
*/
export function isUserTokenValid(type: "access-token" | "refresh-token") {
  const {accessToken, refreshToken} = getCredentialsFromCookies();

  if (type === "access-token") {
    const token = decodeJWT(accessToken || "");
    return token !== null && Date.now() < token.exp! * 1000;
  } else if (type === "refresh-token") {
    const token = decodeJWT(refreshToken || "");
    return token !== null && Date.now() < token.exp! * 1000;
  }
}

export function isUserLoggedIn() {
  return isUserTokenValid("access-token") || isUserTokenValid("refresh-token");
}

export function getUserPermissions(): string[] {
  const {permissions} = getCredentialsFromCookies();
  // "W10=" is equal to "[]"
  return JSON.parse(atob(permissions || "W10="));
}
