import {notFound} from "next/navigation";
import {cookies, headers} from "next/headers";
import {serialize} from "cookie";
import axios, {
  AxiosError,
  AxiosResponse,
  InternalAxiosRequestConfig,
} from "axios";
import {isUserTokenValid} from "@/lib/auth/server";
import {getCredentialsFromCookies} from "@/lib/http";
import {REFRESH_TOKEN_URL} from "./auth";
import {
  INTERNAL_BACKEND_URL,
  ACCESS_TOKEN_EXP,
  REFRESH_TOKEN_EXP,
  ACCESS_TOKEN_COOKIE_NAME,
  REFRESH_TOKEN_COOKIE_NAME,
  PERMISSION_DENIED,
} from "@/constants";

const BASE_URL = `${INTERNAL_BACKEND_URL}/api`;

export const apiClient = axios.create({
  baseURL: BASE_URL,
  headers: {
    "Content-Type": "application/json",
  },
});

function handleRequestResolve(config: InternalAxiosRequestConfig) {
  const accessToken = cookies().get(ACCESS_TOKEN_COOKIE_NAME)?.value;
  if (accessToken !== undefined && config.headers.Authorization === undefined) {
    config.headers.Authorization = `Bearer ${accessToken}`;
  }
  return config;
}

apiClient.interceptors.request.use(
  handleRequestResolve,
  async (error) => error,
);

async function handleResponseRejection(response: AxiosResponse) {
  const headersStore = headers();
  const isFromApiRoutes = Boolean(headersStore.get("client-to-proxy"));
  const isFromServerAction = Boolean(headersStore.get("next-action"));
  const originalRequest = response.config;

  if (response instanceof AxiosError && response.status === 401) {
    const isAccessTokenValid = isUserTokenValid("access-token");
    const {refreshToken} = getCredentialsFromCookies();

    if (isAccessTokenValid) {
      /**
       * If a user with a valid access token encounters a 401 error,
       * it signifies that they lack the necessary permissions to access
       * the requested resource. In this case, refreshing the token is
       * unnecessary, and we should immediately throw a 'permission denied' error.
       */
      throw new Error(PERMISSION_DENIED);
    }

    try {
      const response = await axios.post(`${BASE_URL}/${REFRESH_TOKEN_URL}`, {
        token: refreshToken,
      });
      const {access_token, refresh_token} = response.data;
      originalRequest.headers.Authorization = `Bearer ${access_token}`;
      const originalRequestResponse = await axios(originalRequest);

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
      if (err instanceof AxiosError && err.status === 401) {
        throw new Error(PERMISSION_DENIED);
      }
      return err;
    }
  }

  if (response instanceof AxiosError && response.status === 404) {
    notFound();
  }

  if (response instanceof AxiosError) {
    throw new AxiosError(
      response.message,
      response.code,
      response.config,
      response.request,
      response.response,
    );
  }

  throw new Error("Something bad happened!");
}

apiClient.interceptors.response.use((value) => value, handleResponseRejection);
