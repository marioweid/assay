import { normalizeProblem } from "@/api/errors";
import { client } from "@/api/generated/client.gen";

let requestInterceptor: number | undefined;
let errorInterceptor: number | undefined;

export function configureClient(getToken: () => string | null, onUnauthorized?: () => void): void {
  client.setConfig({
    baseUrl: window.location.origin,
    throwOnError: true,
  });
  if (requestInterceptor !== undefined) {
    client.interceptors.request.eject(requestInterceptor);
  }
  if (errorInterceptor !== undefined) {
    client.interceptors.error.eject(errorInterceptor);
  }
  requestInterceptor = client.interceptors.request.use((request) => {
    const token = getToken();
    if (token === null || token === "") {
      return request;
    }
    const headers = new Headers(request.headers);
    headers.set("Authorization", `Bearer ${token}`);
    return new Request(request, { headers });
  });
  errorInterceptor = client.interceptors.error.use((error, response, request, options) => {
    const current = getToken();
    if (
      response?.status === 401 &&
      current !== null &&
      request?.headers.get("Authorization") === `Bearer ${current}`
    ) {
      onUnauthorized?.();
    }
    const operation = request === undefined ? options.url : `${request.method} ${options.url}`;
    return normalizeProblem(error, response, operation);
  });
}
