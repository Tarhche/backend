import {apiClient} from ".";

export async function loginUser(identity: string, password: string) {
  const response = await apiClient.post("auth/login", {
    identity: identity,
    password: password,
  });

  return response.data;
}

export async function registerUser(identity: string) {
  return await apiClient.post("auth/register", {
    identity: identity,
  });
}

export async function verifyUser(data: Record<string, string>) {
  return await apiClient.post("auth/verify", data);
}

export const REFRESH_TOKEN_URL = "auth/token/refresh";
export async function refreshToken(refreshToken: string) {
  return await apiClient.post(REFRESH_TOKEN_URL, {
    token: refreshToken,
  });
}

export async function forgotPassword(identity: string) {
  return await apiClient.post("auth/password/forget", {
    identity,
  });
}

export async function resetPassword(password: string, token: string) {
  return await apiClient.post("auth/password/reset", {
    password,
    token,
  });
}
