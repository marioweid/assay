import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import type { ConversationCall } from "@/features/traces/conversation-model";
import { ConversationView } from "@/features/traces/conversation-view";

function call(
  id: number,
  input: ConversationCall["input"],
  output: ConversationCall["output"],
): ConversationCall {
  return {
    spanKey: `${id}@2026-09-08T00:00:0${id}Z`,
    span: {
      id,
      name: `model-${id}`,
      input_tokens: 10,
      output_tokens: 20,
      otel_span_id: String(id),
      kind: "client",
      status_code: "ok",
      is_scorable: id === 1,
      start_time: `2026-09-08T00:00:0${id}Z`,
      end_time: `2026-09-08T00:00:0${id}Z`,
      duration_ms: 0,
      children: [],
      events: [],
      attributes: {},
    },
    input,
    output,
  };
}

const input = {
  state: "present" as const,
  diagnostics: [],
  raw: [],
  messages: [
    {
      key: "1@2026-09-08T00:00:01Z:input:0",
      role: "user",
      direction: "input" as const,
      parts: [{ kind: "text" as const, text: "Where is Assay data stored?" }],
      raw: {},
    },
  ],
};

const output = {
  state: "present" as const,
  diagnostics: [],
  raw: [],
  messages: [
    {
      key: "1@2026-09-08T00:00:01Z:output:0",
      role: "assistant",
      direction: "output" as const,
      parts: [
        { kind: "text" as const, text: "In Postgres." },
        { kind: "tool-call" as const, id: "call-1", name: "lookup", arguments: { q: "Assay" } },
      ],
      raw: {},
    },
  ],
};

test("renders a selected call in message order and selects its source", async () => {
  const select = vi.fn();
  render(
    <ConversationView
      calls={[call(1, input, output)]}
      onSelectSpan={select}
      selectedSpanKey={null}
    />,
  );

  expect(screen.getByText("Where is Assay data stored?")).toBeInTheDocument();
  expect(screen.getByText("In Postgres.")).toBeInTheDocument();
  expect(screen.getByText(/Tool call: lookup/)).toBeInTheDocument();

  await userEvent.click(screen.getByRole("button", { name: "View source for assistant message" }));
  expect(select).toHaveBeenCalledWith("1@2026-09-08T00:00:01Z");
});

test("filters by the exact selected span rather than its descendants", () => {
  const second = call(2, input, output);
  render(
    <ConversationView
      calls={[call(1, input, output), second]}
      onSelectSpan={vi.fn()}
      selectedSpanKey={second.spanKey}
    />,
  );

  expect(screen.getByText("model-2")).toBeInTheDocument();
  expect(screen.queryByText("model-1")).not.toBeInTheDocument();
});

test("shows malformed capture as text without executing it", () => {
  const malformed = {
    state: "malformed" as const,
    diagnostics: ["Invalid JSON"],
    raw: "<script>bad()</script>",
    messages: [],
  };
  render(
    <ConversationView
      calls={[call(1, malformed, malformed)]}
      onSelectSpan={vi.fn()}
      selectedSpanKey={null}
    />,
  );

  expect(screen.getByText("Could not parse captured messages")).toBeInTheDocument();
  expect(screen.getByText(/<script>bad\(\)<\/script>/)).toBeInTheDocument();
  expect(document.querySelector("script")).toBeNull();
});
