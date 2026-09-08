import { describe, expect, test } from "vitest";

import type { EvalRunResponse } from "@/api/generated/types.gen";
import { isActiveRun } from "@/features/runs/run-status";

describe("isActiveRun", () => {
  const cases: Array<[EvalRunResponse["status"], boolean]> = [
    ["pending", true],
    ["running", true],
    ["succeeded", false],
    ["failed", false],
    ["canceled", false],
  ];

  test.each(cases)("returns %s for %s", (status, expected) => {
    expect(isActiveRun(status)).toBe(expected);
  });
});
