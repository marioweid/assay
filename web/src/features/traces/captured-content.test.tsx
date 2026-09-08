import { render, screen } from "@testing-library/react";
import type { SpanResponse } from "@/api/generated/types.gen";
import { CapturedContent } from "@/features/traces/captured-content";

test("preserves malformed captured text without interpreting HTML", () => {
  const span: SpanResponse = {
    id: 1,
    name: "generation",
    input_tokens: 0,
    output_tokens: 0,
    otel_span_id: "01",
    kind: "internal",
    status_code: "ok",
    is_scorable: false,
    start_time: "2026-09-07T10:00:00Z",
    end_time: "2026-09-07T10:00:01Z",
    duration_ms: 1000,
    children: [],
    events: [],
    attributes: { "gen_ai.input.messages": "[<script>untrusted input" },
  };
  render(<CapturedContent spans={[span]} />);
  expect(screen.getByText("[<script>untrusted input")).toBeInTheDocument();
  expect(document.querySelector("script")).toBeNull();
});

test("explains when no content was captured", () => {
  render(<CapturedContent spans={[]} />);
  expect(screen.getByText("No input or output captured.")).toBeInTheDocument();
});
