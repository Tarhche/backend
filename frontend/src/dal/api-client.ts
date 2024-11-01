import {serialize} from "cookie";
import {cookies as nextCookies, headers} from "next/headers";
import axios, {AxiosError} from "axios";
import {apiPaths} from "./api-paths";
import {INTERNAL_BACKEND_URL} from "@/constants/envs";
import {ACCESS_TOKEN_EXP, REFRESH_TOKEN_EXP} from "@/constants/numbers";

const BASE_URL = `${INTERNAL_BACKEND_URL}/api`;

export const apiClient = axios.create({
  baseURL: BASE_URL,
  headers: {
    "Content-Type": "application/json",
  },
});

apiClient.interceptors.request.use(
  async (config) => {
    const accessToken = nextCookies().get("access_token")?.value;
    if (accessToken !== undefined) {
      config.headers.Authorization = `Bearer ${accessToken}`;
    }
    return config;
  },
  async (error) => error,
);

apiClient.interceptors.response.use(
  (value) => value,
  async (error) => {
    const headersStore = headers();
    const cookiesStore = nextCookies();
    const originalRequest = error.config;
    const isFromApiRoutes = Boolean(headersStore.get("client-to-proxy"));
    if (error instanceof AxiosError && error.status === 401) {
      const refreshToken = cookiesStore.get("refresh_token")?.value;
      if (refreshToken === undefined || originalRequest._retry) {
        return error;
      }
      try {
        const response = await axios.post(
          `${BASE_URL}/${apiPaths.auth.refreshToken}`,
          {
            token: refreshToken,
          },
        );
        const {access_token, refresh_token} = response.data;
        const originalRequestResponse = await axios({
          ...originalRequest,
          headers: {
            Authorization: `Bearer ${access_token}`,
          },
        });
        if (isFromApiRoutes) {
          originalRequestResponse.headers["set-cookie"] = [
            serialize("access_token", access_token, {
              httpOnly: true,
              maxAge: ACCESS_TOKEN_EXP,
              path: "/",
            }),
            serialize("refresh_token", refresh_token, {
              httpOnly: true,
              maxAge: REFRESH_TOKEN_EXP,
              path: "/",
            }),
          ];
          return originalRequestResponse;
        }
      } catch (err) {
        if (err instanceof AxiosError) {
        }
        if (isFromApiRoutes) {
          return err;
        }
      }
    }
    return error;
  },
);
