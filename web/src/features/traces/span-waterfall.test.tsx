import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { SpanResponse } from "@/api/generated/types.gen";
import { SpanWaterfall } from "@/features/traces/span-waterfall";

function span(id: number, start: string, end: string, children: SpanResponse[] = []): SpanResponse {
  return {
    id,
    name: `span-${id}`,
    input_tokens: 0,
    output_tokens: 0,
    otel_span_id: String(id),
    kind: "client",
    status_code: "ok",
    is_scorable: false,
    start_time: start,
    end_time: end,
    duration_ms: 0,
    children,
    events: [],
    attributes: {},
  };
}

test("selects an accessible waterfall row without relying on color", async () => {
  const select = vi.fn();
  render(
    <SpanWaterfall
      onSelect={select}
      spans={[span(1, "2026-09-08T00:00:00Z", "2026-09-08T00:00:01Z")]}
    />,
  );

  await userEvent.click(screen.getByRole("button", { name: /span-1.*1000/ }));
  expect(select).toHaveBeenCalledWith("1@2026-09-08T00:00:00Z");
  expect(screen.getByLabelText("Timeline for span-1")).toBeInTheDocument();
});
