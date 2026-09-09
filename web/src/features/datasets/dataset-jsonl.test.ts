import { expect, test } from "vitest";

import { parseDatasetJsonl, serializeDatasetJsonl } from "@/features/datasets/dataset-jsonl";

test("reports the failing JSONL line without echoing content", () => {
  const source = '{"input":{"question":"ok"}}\n{private-invalid';
  expect(() => parseDatasetJsonl(source)).toThrow("Line 2: invalid JSON");
});

test("accepts BOM, CRLF, and blank lines", () => {
  expect(
    parseDatasetJsonl('\uFEFF{"input":{"question":"one"}}\r\n\r\n{"input":{"question":"two"}}'),
  ).toEqual([{ input: { question: "one" } }, { input: { question: "two" } }]);
});

test("rejects invalid records and duplicate external IDs", () => {
  expect(() => parseDatasetJsonl('{"input":[]}')).toThrow("Line 1: invalid dataset item");
  expect(() =>
    parseDatasetJsonl('{"external_id":"same","input":{}}\n{"external_id":"same","input":{}}'),
  ).toThrow("Line 2: duplicate external_id");
});

test("exports editable fields without database identifiers", () => {
  const text = serializeDatasetJsonl([
    {
      context: [],
      created_at: "2026-09-08T00:00:00Z",
      dataset_id: "dataset",
      external_id: "case-1",
      id: "item",
      input: { question: "Why?" },
      metadata: {},
      updated_at: "2026-09-08T00:00:00Z",
    },
  ]);
  expect(JSON.parse(text)).toEqual({
    context: [],
    external_id: "case-1",
    input: { question: "Why?" },
    metadata: {},
  });
});
