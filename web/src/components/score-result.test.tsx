import { render, screen } from "@testing-library/react";

import type { ScoreResponse } from "@/api/generated/types.gen";
import { ScoreResult } from "@/components/score-result";

const zeroScore = {
  id: 1,
  scorer: "groundedness",
  value: 0,
  threshold: 0.5,
  passed: false,
  rationale: "No claims were supported.",
  details: {},
  prompt_template_id: "groundedness@v1",
  judge_model: "judge",
  judge_provider: "fake",
  judge_tokens: 12,
  eval_run_id: "019d11d2-cbd3-7a5e-ae83-9b791c932922",
  dataset_item_id: "019d11d2-cbd3-7a5e-ae83-9b791c932933",
  created_at: "2026-09-20T07:00:00Z",
} satisfies ScoreResponse;

test("renders a zero score as a failed score", () => {
  render(<ScoreResult score={zeroScore} />);

  expect(screen.getByText("0.00")).toBeInTheDocument();
  expect(screen.getByText(/Fail/)).toBeInTheDocument();
  expect(screen.queryByText("Not scored")).not.toBeInTheDocument();
});
