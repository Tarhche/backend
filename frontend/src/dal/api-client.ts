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
import {
  INTERNAL_BACKEND_URL,
  ACCESS_TOKEN_EXP,
  REFRESH_TOKEN_EXP,
  ACCESS_TOKEN_COOKIE_NAME,
  REFRESH_TOKEN_COOKIE_NAME,
} from "@/constants";
import {REFRESH_TOKEN_URL} from "./auth";
import {APIClientError, APIClientUnauthorizedError} from "./api-client-errors";

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
  const isAccessTokenValid = isUserTokenValid("access-token");
  const isRefreshTokenValid = isUserTokenValid("refresh-token");
  const isResponseUnauthorized = response.status === 401;
  const originalRequest = response.config;

  const unauthorizedError = new APIClientUnauthorizedError(
    `A user tried to access ${response.config.url} but encountered 401`,
    response.data,
  );
  const unexpectedBehaviorError = new APIClientError(
    "Something bad happened",
    500,
    response.data,
  );

  if (isResponseUnauthorized && isAccessTokenValid) {
    /*
     * If a user with a valid access token encounters a 401 error,
     * it signifies that they lack the necessary permissions to access the
     * requested resource. In this case, refreshing the token is unnecessary,
     * and we should immediately throw a 'APIClientUnauthorizedError'.
     */
    throw unauthorizedError;
  }

  if (isResponseUnauthorized && isRefreshTokenValid) {
    /*
     If a user's access token is invalid but a valid refresh token is available, 
     we should attempt to obtain a new access token and retry the original request 
     with the updated token.
     */
    const {refreshToken} = getCredentialsFromCookies();
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
        /*
         * If the user still receives a 401 error after obtaining a new access token,
         * it indicates they lack the necessary permissions for requested resource.
         */
        throw unauthorizedError;
      }

      if (err instanceof AxiosError) {
        throw new APIClientError(err.message, err.status || 500, response.data);
      }
      throw unexpectedBehaviorError;
    }
  }

  if (
    isResponseUnauthorized &&
    isAccessTokenValid === false &&
    isRefreshTokenValid === false
  ) {
    /*
     * If the user encounters a 401 Unauthorized error and neither
     * an access token nor a refresh token is available, a 401 error should be
     * thrown indicating that authentication is required.
     */
    throw unauthorizedError;
  }

  if (response.status === 404) {
    notFound();
  }

  throw new APIClientError("", response.status, response.data);
}

apiClient.interceptors.response.use((value) => value, handleResponseRejection);
