import {AxiosResponse} from "axios";

export function axiosToFetchResponse(
  axiosResponse: AxiosResponse,
  customBody?: any,
) {
  const {data, status, headers} = axiosResponse;

  const headersObj = new Headers();
  if (headers && headers["set-cookie"]) {
    headers["set-cookie"].forEach((cookie) => {
      headersObj.append("set-cookie", cookie);
    });
  }

  const body = new Blob([JSON.stringify(customBody ?? data)], {
    type: "application/json",
  });

  return new Response(body, {
    status,
    statusText: axiosResponse.statusText,
    headers: headersObj,
  });
}

export function convertFormDataActionToObject(formData: FormData) {
  const object: Record<string, string | string[]> = {};

  formData.forEach((value, key) => {
    if (value.toString() && key.includes("$") === false) {
      object[key] = value.toString();
    }
  });

  return object;
}
