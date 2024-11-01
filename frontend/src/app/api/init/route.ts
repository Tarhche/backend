import {NextRequest, NextResponse} from "next/server";
import {AxiosError} from "axios";
import {fetchUserProfile} from "@/dal/profile";
import {AuthState} from "@/types/api-responses/init";
import {axiosToFetchResponse} from "@/lib/transformers";

export async function GET(request: NextRequest) {
  try {
    const profile = await fetchUserProfile();
    if (profile.status === 400) {
      throw new AxiosError("", "400");
    }
    if (profile.status === 200) {
      const data: AuthState = {
        status: "authenticated",
        profile: profile.data,
      };
      return axiosToFetchResponse(profile, data);
    } else if (profile.status === 401) {
      const data: AuthState = {
        status: "unauthenticated",
      };
      return new Response(JSON.stringify(data), {
        status: 200,
      });
    }
  } catch {
    const response = new NextResponse(null, {
      status: 401,
    });
    const cookies = request.cookies.getAll();
    cookies.forEach(({name}) => {
      response.cookies.set(name, "", {maxAge: -1});
    });
    return response;
  }
}
