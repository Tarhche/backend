/* 
    This is not meant to be used on the server.
*/

export async function fetchWrapper(
  input: string | URL | globalThis.Request,
  init?: RequestInit,
) {
  const options: RequestInit = {
    ...init,
    headers: {
      "Content-Type": "application/json",
      "Client-To-Proxy": "TRUE",
      ...init?.headers,
    },
  };
  const response = await fetch(input, options);
  if (!response.ok) {
    if (response.status === 401) {
      const currentUrl = `${window.location.pathname}${window.location.search}`;
      window.location.href = `/auth/login?callbackUrl=${encodeURIComponent(currentUrl)}`;
      return;
    }
    const error = await response.json();
    throw new Error(error.message || "Fetch request failed");
  }
  return await response.json();
}
