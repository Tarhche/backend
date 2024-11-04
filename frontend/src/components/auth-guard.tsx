import {cookies} from "next/headers";
import {type ReactNode} from "react";
import jwt from "jsonwebtoken";

type Props = {
  children: ReactNode;
  fallback?: ReactNode;
};

export function AuthGuard({fallback, children}: Props) {
  const cookiesStore = cookies();
  const refreshToken = cookiesStore.get("refresh_token")?.value;
  const decodedRefreshToken = jwt.decode(refreshToken ?? "", {
    json: true,
  });

  if (
    decodedRefreshToken === null ||
    Date.now() > new Date(decodedRefreshToken.exp! * 1000).getTime()
  ) {
    return fallback ?? null;
  }

  return children;
}
