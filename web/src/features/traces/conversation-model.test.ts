import type { SpanResponse } from "@/api/generated/types.gen";
import {
  conversationCalls,
  normalizeMessages,
  spanKey,
} from "@/features/traces/conversation-model";
import standardMessages from "@/features/traces/fixtures/conversation.json";

function span(
  id: number,
  startTime: string,
  attributes: Record<string, unknown> = {},
  children: SpanResponse[] = [],
): SpanResponse {
  return {
    id,
    name: `span-${id}`,
    input_tokens: 0,
    output_tokens: 0,
    otel_span_id: id.toString(16).padStart(16, "0"),
    kind: "internal",
    status_code: "ok",
    is_scorable: false,
    start_time: startTime,
    end_time: startTime,
    duration_ms: 0,
    children,
    events: [],
    attributes,
  };
}

test("reads ordered standard text parts", () => {
  const result = normalizeMessages(
    JSON.stringify([
      {
        role: "user",
        parts: [
          { type: "text", content: "first" },
          { type: "text", content: "second" },
        ],
      },
    ]),
    "input",
  );

  expect(result.state).toBe("present");
  expect(result.messages[0]?.parts).toEqual([
    { kind: "text", text: "first" },
    { kind: "text", text: "second" },
  ]);
});

test("distinguishes absent, empty, and malformed capture", () => {
  expect(normalizeMessages(undefined, "input").state).toBe("missing");
  expect(normalizeMessages([], "input").state).toBe("empty");
  expect(normalizeMessages("[broken", "input").state).toBe("malformed");
});

test("normalizes structured and JSON-encoded standard messages identically", () => {
  const messages = standardMessages.standard_text_parts;
  const structured = normalizeMessages(messages, "input");
  const encoded = normalizeMessages(JSON.stringify(messages), "input");

  expect(structured.state).toBe(encoded.state);
  expect(structured.messages).toEqual(encoded.messages);
  expect(structured.diagnostics).toEqual(encoded.diagnostics);
});

test("preserves tool parts, unfamiliar parts, duplicate messages, and plain HTML", () => {
  const result = normalizeMessages(
    [
      ...standardMessages.tool_parts,
      { role: "developer", content: "<script>do not execute()</script>" },
      { role: "assistant", parts: [{ type: "attachment", url: "https://example.test/file" }] },
      { role: "user", content: "repeat" },
      { role: "user", content: "repeat" },
    ],
    "output",
  );

  expect(result.state).toBe("present");
  expect(result.messages.map((message) => message.role)).toEqual([
    "assistant",
    "tool",
    "developer",
    "assistant",
    "user",
    "user",
  ]);
  expect(result.messages[0]?.parts[0]).toEqual({
    kind: "tool-call",
    id: "call-1",
    name: "lookup",
    arguments: { query: "Assay" },
  });
  expect(result.messages[1]?.parts[0]).toEqual({
    kind: "tool-result",
    id: "call-1",
    result: { answer: "An evaluation workspace." },
  });
  expect(result.messages[2]?.parts[0]).toEqual({
    kind: "text",
    text: "<script>do not execute()</script>",
  });
  expect(result.messages[3]?.parts[0]).toMatchObject({ kind: "unsupported", type: "attachment" });
  expect(result.messages[4]?.key).not.toBe(result.messages[5]?.key);
});

test("keeps valid siblings when one message is malformed", () => {
  const result = normalizeMessages(
    [
      { role: "user", content: "valid" },
      { role: 42 },
      { role: "assistant", content: "also valid" },
    ],
    "input",
  );

  expect(result.state).toBe("present");
  expect(result.messages.map((message) => message.role)).toEqual(["user", "assistant"]);
  expect(result.diagnostics).toHaveLength(1);
  expect(result.raw).toEqual([
    { role: "user", content: "valid" },
    { role: 42 },
    { role: "assistant", content: "also valid" },
  ]);
});

test("retains valid parts when a sibling tool call is malformed", () => {
  const result = normalizeMessages(
    [
      {
        role: "assistant",
        parts: [
          { type: "text", content: "answer" },
          { type: "tool_call", id: "call-1" },
        ],
      },
    ],
    "output",
  );

  expect(result.messages[0]?.parts).toEqual([{ kind: "text", text: "answer" }]);
  expect(result.diagnostics).toHaveLength(1);
});

test("groups message-bearing spans by stable source identity and start time", () => {
  const child = span(3, "2026-09-08T00:00:01Z", {
    "gen_ai.input.messages": [{ role: "user", content: "first" }],
    "gen_ai.output.messages": [],
  });
  const late = span(2, "2026-09-08T00:00:02Z", { "gen_ai.input.messages": [] });
  const early = span(1, "2026-09-08T00:00:00Z", {}, [child]);

  const calls = conversationCalls([late, early, child]);

  expect(calls.map((call) => call.span.id)).toEqual([3, 2]);
  expect(calls[0]?.spanKey).toBe(spanKey(child));
  expect(calls[0]?.output.messages).toEqual([]);
  expect(calls[0]?.output.state).toBe("empty");
  expect(calls[0]?.input.messages[0]?.key).toBe(`${spanKey(child)}:input:0`);
});

test("does not coerce malformed top-level values into messages", () => {
  for (const value of [null, {}, "{}", 3]) {
    const result = normalizeMessages(value, "input");
    expect(result.state).toBe("malformed");
    expect(result.messages).toEqual([]);
  }
});
