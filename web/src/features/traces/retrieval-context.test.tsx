import { render, screen } from "@testing-library/react";
import type { SpanResponse } from "@/api/generated/types.gen";
import { RetrievalContext } from "@/features/traces/retrieval-context";

function span(attributes: Record<string, unknown>): SpanResponse {
  return {
    id: 1,
    name: "retrieval",
    input_tokens: 0,
    output_tokens: 0,
    otel_span_id: "01",
    kind: "client",
    status_code: "ok",
    is_scorable: false,
    start_time: "2026-09-08T00:00:00Z",
    end_time: "2026-09-08T00:00:00Z",
    duration_ms: 0,
    children: [],
    events: [],
    attributes,
  };
}

test("uses flattened Assay chunks when standard retrieval documents are absent", () => {
  render(
    <RetrievalContext
      spans={[
        span({
          "assay.context.chunk.count": 1,
          "assay.context.chunks.0.id": "manual",
          "assay.context.chunks.0.text": "Assay stores evidence.",
        }),
      ]}
    />,
  );

  expect(screen.getByText("Retrieved context")).toBeInTheDocument();
  expect(screen.getByText(/Assay stores evidence/)).toBeInTheDocument();
});
