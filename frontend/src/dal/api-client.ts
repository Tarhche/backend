import {notFound} from "next/navigation";
import {cookies, headers} from "next/headers";
import {serialize} from "cookie";
import axios, {AxiosError} from "axios";
import {REFRESH_TOKEN_URL} from "./auth";
import {INTERNAL_BACKEND_URL} from "@/constants/envs";
import {ACCESS_TOKEN_EXP, REFRESH_TOKEN_EXP} from "@/constants/numbers";
import {
  ACCESS_TOKEN_COOKIE_NAME,
  REFRESH_TOKEN_COOKIE_NAME,
} from "@/constants/strings";

const BASE_URL = `${INTERNAL_BACKEND_URL}/api`;

export const apiClient = axios.create({
  baseURL: BASE_URL,
  headers: {
    "Content-Type": "application/json",
  },
});

apiClient.interceptors.request.use(
  async (config) => {
    const accessToken = cookies().get(ACCESS_TOKEN_COOKIE_NAME)?.value;
    if (
      accessToken !== undefined &&
      config.headers.Authorization === undefined
    ) {
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
    const cookiesStore = cookies();
    const originalRequest = error.config;
    const isFromApiRoutes = Boolean(headersStore.get("client-to-proxy"));
    const isFromServerAction = Boolean(headersStore.get("next-action"));
    if (error instanceof AxiosError && error.status === 401) {
      const refreshToken = cookiesStore.get(REFRESH_TOKEN_COOKIE_NAME)?.value;
      if (refreshToken === undefined || originalRequest._retry) {
        return error;
      }
      try {
        const response = await axios.post(`${BASE_URL}/${REFRESH_TOKEN_URL}`, {
          token: refreshToken,
        });
        const {access_token, refresh_token} = response.data;
        originalRequest._retry = true;
        const originalRequestResponse = await axios({
          ...originalRequest,
          headers: {
            Authorization: `Bearer ${access_token}`,
          },
        });
        if (isFromApiRoutes) {
          originalRequestResponse.headers["set-cookie"] = [
            serialize(ACCESS_TOKEN_COOKIE_NAME, access_token, {
              httpOnly: true,
              maxAge: ACCESS_TOKEN_EXP,
              path: "/",
            }),
            serialize(REFRESH_TOKEN_COOKIE_NAME, refresh_token, {
              httpOnly: true,
              maxAge: REFRESH_TOKEN_EXP,
              path: "/",
            }),
          ];
          return originalRequestResponse;
        }
        if (isFromServerAction) {
          cookies().set(ACCESS_TOKEN_COOKIE_NAME, access_token, {
            httpOnly: true,
            maxAge: ACCESS_TOKEN_EXP,
            path: "/",
          });
          cookies().set(REFRESH_TOKEN_COOKIE_NAME, refresh_token, {
            httpOnly: true,
            maxAge: REFRESH_TOKEN_EXP,
            path: "/",
          });
        }
        return originalRequestResponse;
      } catch (err) {
        return err;
      }
    }
    if (error instanceof AxiosError && error.status === 404) {
      notFound();
    }
    if (error instanceof AxiosError) {
      throw new AxiosError(
        error.message,
        error.code,
        error.config,
        error.request,
        error.response,
      );
    }
    throw new Error("Something bad happened!");
  },
);
