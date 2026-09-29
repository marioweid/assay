import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";

import type { SessionTurnResponse } from "@/api/generated/types.gen";
import { SessionThread, turnDuration } from "@/features/sessions/session-thread";

const start = "2026-09-01T10:00:00Z";

test.each([
  [start, start, "0 ms"],
  [start, "2026-09-01T10:00:00.250Z", "250 ms"],
  [start, "2026-09-01T10:00:02Z", "2s"],
  [start, "2026-09-01T09:59:59Z", "Timing unavailable"],
  ["invalid", start, "Timing unavailable"],
  [start, "", "Timing unavailable"],
])("turn duration handles %s to %s", (from, to, expected) => {
  expect(turnDuration(from, to)).toBe(expected);
});

test("rich message controls remain separate from the source-selection button", async () => {
  const turn: SessionTurnResponse = {
    id: "source",
    root_name: "answer",
    start_time: start,
    end_time: start,
    span_count: 1,
    total_tokens: 0,
    status: "ok",
    attributes: {
      "gen_ai.output.messages": [
        {
          role: "assistant",
          parts: [{ type: "tool_call", name: "lookup", arguments: { query: "docs" } }],
        },
      ],
    },
  };
  let selected = "";
  render(
    <MemoryRouter>
      <SessionThread
        appId="demo"
        turns={[turn]}
        selectedId={null}
        onSelect={(id) => {
          selected = id;
        }}
      />
    </MemoryRouter>,
  );
  const bubble = screen.getByRole("button", { name: /View captured assistant message/ });
  expect(bubble.querySelector("button, summary, a")).toBeNull();
  const user = userEvent.setup();
  await user.click(bubble);
  expect(selected).toBe("source");
  await user.click(screen.getByText("Message tools and attachments"));
  expect(screen.getByText(/Tool call: lookup/)).toBeVisible();
});
