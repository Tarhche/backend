import {NextRequest, NextResponse} from "next/server";
import jwt from "jsonwebtoken";
import {refreshToken as getNewTokens} from "./dal/public/auth";
import {
  ACCESS_TOKEN_COOKIE_NAME,
  REFRESH_TOKEN_COOKIE_NAME,
} from "@/constants/strings";
import {ACCESS_TOKEN_EXP, REFRESH_TOKEN_EXP} from "./constants/numbers";

export async function middleware(request: NextRequest) {
  const accessToken = request.cookies.get(ACCESS_TOKEN_COOKIE_NAME)?.value;
  const refreshToken = request.cookies.get(REFRESH_TOKEN_COOKIE_NAME)?.value;

  try {
    const decodedAccessToken = jwt.decode(accessToken ?? "", {
      json: true,
    });
    if (
      refreshToken === undefined ||
      (decodedAccessToken === null && refreshToken === undefined)
    ) {
      throw new Error();
    }
    if (
      decodedAccessToken === null ||
      (decodedAccessToken !== null &&
        Date.now() > decodedAccessToken.exp! * 1000)
    ) {
      try {
        const newTokens = (await getNewTokens(refreshToken!)).data;
        const nextResponse = NextResponse.next();
        nextResponse.cookies.set(
          ACCESS_TOKEN_COOKIE_NAME,
          newTokens.access_token,
          {
            httpOnly: true,
            maxAge: ACCESS_TOKEN_EXP,
            path: "/",
          },
        );
        nextResponse.cookies.set(
          REFRESH_TOKEN_COOKIE_NAME,
          newTokens.refresh_token,
          {
            httpOnly: true,
            maxAge: REFRESH_TOKEN_EXP,
            path: "/",
          },
        );
        return nextResponse;
      } catch {
        throw new Error();
      }
    }
  } catch {
    const {origin, pathname} = request.nextUrl;
    const url = new URL(`${origin}/auth/login?callbackUrl=${pathname}`);
    const redirectResponse = NextResponse.redirect(url);
    const cookies = request.cookies.getAll();
    cookies.forEach(({name}) => {
      redirectResponse.cookies.set(name, "", {maxAge: -1});
    });
    return redirectResponse;
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/dashboard/:path*"],
};
