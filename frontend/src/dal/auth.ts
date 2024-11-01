import {apiPaths, apiClient} from ".";

export async function loginUser(identity: string, password: string) {
  const response = await apiClient.post(apiPaths.auth.login, {
    identity: identity,
    password: password,
  });

  return response.data;
}

export async function refreshToken(refreshToken: string) {
  return await apiClient.post(apiPaths.auth.refreshToken, {
    token: refreshToken,
  });
}
