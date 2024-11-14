import {NextRequest} from "next/server";
import {AxiosError} from "axios";
import {apiClient} from "@/dal/api-client";
import {axiosToFetchResponse} from "@/lib/transformers";

export async function GET(request: NextRequest, {params}) {
  return handleRequest(request, params, "GET");
}

export async function POST(request: NextRequest, {params}) {
  return handleRequest(request, params, "POST");
}

export async function PUT(request: NextRequest, {params}) {
  return handleRequest(request, params, "PUT");
}

export async function PATCH(request: NextRequest, {params}) {
  return handleRequest(request, params, "PATCH");
}

export async function DELETE(request: NextRequest, {params}) {
  return handleRequest(request, params, "DELETE");
}

async function handleRequest(request: NextRequest, params, method: string) {
  try {
    const response = await apiClient({
      url: params.proxy.join("/") + request.nextUrl.search,
      method: method,
      params: request.nextUrl.searchParams,
      data: request.body,
    });
    return axiosToFetchResponse(response);
  } catch (error) {
    if (error instanceof AxiosError) {
      return new Response(JSON.stringify(error.response?.data), {
        status: error.status,
      });
    }
    return new Response(JSON.stringify({error: "Can't handle your request"}), {
      status: 400,
    });
  }
}
