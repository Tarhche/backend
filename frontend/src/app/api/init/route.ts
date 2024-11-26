import {NextRequest, NextResponse} from "next/server";
import {fetchUserProfile} from "@/dal";
import {AuthState} from "@/types/api-responses/init";
import {axiosToFetchResponse} from "@/lib/transformers";
import {APIClientUnauthorizedError} from "@/dal/api-client-errors";

export async function GET(request: NextRequest) {
  try {
    const profile = await fetchUserProfile();
    const data: AuthState = {
      status: "authenticated",
      profile: profile.data,
    };

    return axiosToFetchResponse(profile, data);
  } catch (err) {
    if (err instanceof APIClientUnauthorizedError) {
      return new Response(
        JSON.stringify({
          status: "unauthenticated",
        }),
        {
          status: 200,
        },
      );
    }

    const cookies = request.cookies.getAll();
    const response = new NextResponse();
    cookies.forEach(({name}) => {
      response.cookies.set(name, "", {maxAge: -1});
    });
    return response;
  }
}
