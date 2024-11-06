import jwt from "jsonwebtoken";
import {getCredentialsFromCookies} from "./http";

export function decodeCredentials() {
  const {accessToken, refreshToken} = getCredentialsFromCookies();

  return {
    accessToken: jwt.decode(accessToken ?? "", {
      json: true,
    }),
    refreshToken: jwt.decode(refreshToken ?? "", {
      json: true,
    }),
  };
}

export function isUserLoggedIn() {
  const {accessToken, refreshToken} = decodeCredentials();
  const isAccessTokenValid =
    accessToken !== null && Date.now() < accessToken.exp! * 1000;
  const isRefreshTokenValid =
    refreshToken !== null && Date.now() < refreshToken.exp! * 1000;

  return isAccessTokenValid || isRefreshTokenValid;
}
