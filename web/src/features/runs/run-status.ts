import type { EvalRunResponse } from "@/api/generated/types.gen";

export function isActiveRun(status: EvalRunResponse["status"]): boolean {
  return status === "pending" || status === "running";
}
