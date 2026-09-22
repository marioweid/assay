import { HttpResponse, http } from "msw";
import { setupServer } from "msw/node";

import { configureClient } from "@/api/client";
import { collectRunResultsJSONL } from "@/features/runs/run-export";

const runID = "019d11d2-cbd3-7a5e-ae83-9b791c932922";
const server = setupServer();

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => configureClient(() => "admin-secret"));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

test("exports every item page as JSONL", async () => {
  const cursors: Array<string | null> = [];
  server.use(
    http.get(`*/v1/runs/${runID}/items`, ({ request }) => {
      const cursor = new URL(request.url).searchParams.get("cursor");
      cursors.push(cursor);
      return HttpResponse.json({
        items: [{ dataset_item_id: cursor === null ? "first" : "second", scores: [] }],
        ...(cursor === null ? { next_cursor: "next" } : {}),
      });
    }),
  );

  const lines = (await collectRunResultsJSONL(runID)).trim().split("\n");

  expect(lines.map((line) => JSON.parse(line))).toEqual([
    { dataset_item_id: "first", scores: [] },
    { dataset_item_id: "second", scores: [] },
  ]);
  expect(cursors).toEqual([null, "next"]);
});

test("stops an export when the server repeats a cursor", async () => {
  server.use(
    http.get(`*/v1/runs/${runID}/items`, () =>
      HttpResponse.json({ items: [], next_cursor: "same" }),
    ),
  );

  await expect(collectRunResultsJSONL(runID)).rejects.toThrow("server repeated a cursor");
});

test("stops browser exports above 10,000 cases", async () => {
  server.use(
    http.get(`*/v1/runs/${runID}/items`, () =>
      HttpResponse.json({
        items: Array.from({ length: 10_001 }, (_, index) => ({
          dataset_item_id: String(index),
          scores: [],
        })),
      }),
    ),
  );

  await expect(collectRunResultsJSONL(runID)).rejects.toThrow("exceeds 10,000 cases");
});
