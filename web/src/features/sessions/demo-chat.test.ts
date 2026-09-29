import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { waitFor } from "@testing-library/dom";
import userEvent from "@testing-library/user-event";

const chatRoot = resolve(process.cwd(), "../examples/python-qa/static");
const chatScript = readFileSync(resolve(chatRoot, "chat.js"), "utf8");
const chatHtml = readFileSync(resolve(chatRoot, "index.html"), "utf8");

function json(body: object): Response {
  return new Response(JSON.stringify(body), { headers: { "Content-Type": "application/json" } });
}

function startChat(): Promise<void> {
  const run = new Function(`return (async () => { ${chatScript} })()`) as () => Promise<void>;
  return run();
}

beforeEach(() => {
  document.body.innerHTML = new DOMParser().parseFromString(chatHtml, "text/html").body.innerHTML;
});

afterEach(() => {
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

test("new chat cannot race with a pending session restore", async () => {
  let resolveHistory: ((value: Response) => void) | undefined;
  const pendingHistory = new Promise<Response>((resolve) => {
    resolveHistory = resolve;
  });
  const fetcher = vi.fn((url: string, options?: RequestInit): Promise<Response> => {
    if (url === "/api/config") {
      return Promise.resolve(json({ model: "local-model", traces_url: "http://localhost:8080" }));
    }
    if (url === "/api/session" && options?.method === "POST") {
      return Promise.resolve(json({ session_id: "new", turns: [], next_cursor: null }));
    }
    if (url === "/api/session") return pendingHistory;
    throw new Error(`Unexpected request: ${url}`);
  });
  vi.stubGlobal("fetch", fetcher);
  const started = startChat();
  await waitFor(() => expect(fetcher).toHaveBeenCalledWith("/api/session"));
  const newChat = document.querySelector<HTMLButtonElement>("#clear");
  const send = document.querySelector<HTMLButtonElement>("#send");
  expect(newChat?.disabled).toBe(true);
  expect(send?.disabled).toBe(true);
  await userEvent.click(newChat!);
  expect(fetcher).not.toHaveBeenCalledWith("/api/session", { method: "POST" });
  resolveHistory?.(
    json({
      session_id: "old",
      turns: [{ question: "Old question", answer: "Old answer", trace_url: null }],
      next_cursor: null,
    }),
  );
  await started;
  expect(document.querySelector("#messages")?.textContent).toContain("Old answer");
  expect(document.querySelector("#welcome")?.hasAttribute("hidden")).toBe(true);
  expect(document.querySelector("h1")?.textContent).toContain("Python Q&A example");
  await userEvent.click(newChat!);
  await waitFor(() => expect(document.querySelector("#messages")?.textContent).toBe(""));
  expect(fetcher).toHaveBeenCalledWith("/api/session", { method: "POST" });
  expect(send?.disabled).toBe(false);
});

test("failed restore cannot send but New chat can establish a fresh session", async () => {
  const fetcher = vi.fn((url: string, options?: RequestInit): Promise<Response> => {
    if (url === "/api/config") {
      return Promise.resolve(json({ model: "local-model", traces_url: "http://localhost:8080" }));
    }
    if (url === "/api/session" && options?.method === "POST") {
      return Promise.resolve(json({ session_id: "new", turns: [], next_cursor: null }));
    }
    if (url === "/api/session") return Promise.resolve(new Response("{}", { status: 502 }));
    throw new Error(`Unexpected request: ${url}`);
  });
  vi.stubGlobal("fetch", fetcher);
  await startChat();
  const newChat = document.querySelector<HTMLButtonElement>("#clear");
  const send = document.querySelector<HTMLButtonElement>("#send");
  expect(send?.disabled).toBe(true);
  expect(newChat?.disabled).toBe(false);
  await userEvent.click(newChat!);
  await waitFor(() => expect(send?.disabled).toBe(false));
  expect(fetcher).toHaveBeenCalledWith("/api/session", { method: "POST" });
});
