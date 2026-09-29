import { HttpResponse, http } from "msw";

export const tokenModeServerInfo = http.get("*/v1/server-info", () =>
  HttpResponse.json({ local_mode: false }),
);
