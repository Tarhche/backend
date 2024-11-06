import {cookies, headers} from "next/headers";

export function getRootUrl() {
  const host = headers().get("host");
  const protocol = process.env.NODE_ENV === "production" ? "https" : "http";
  return `${protocol}://${host}`;
}

export function getCredentialsFromCookies() {
  const cookiesStore = cookies();
  return {
    accessToken: cookiesStore.get("access_token")?.value,
    refreshToken: cookiesStore.get("refresh_token")?.value,
  };
}
