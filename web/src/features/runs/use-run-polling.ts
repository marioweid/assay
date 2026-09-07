import { useEffect, useState } from "react";

import { Problem } from "@/api/errors";
import { getEvalRun } from "@/api/generated/sdk.gen";
import type { EvalRunResponse } from "@/api/generated/types.gen";

const activeStatuses = new Set(["queued", "running"]);

type RunPolling = {
  error: string | null;
  retry: () => void;
  run: EvalRunResponse | null;
  stopped: boolean;
};

export function useRunPolling(runID: string): RunPolling {
  const [run, setRun] = useState<EvalRunResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [stopped, setStopped] = useState(false);
  const [retryGeneration, setRetryGeneration] = useState(0);

  useEffect(() => {
    let disposed = false;
    let shouldPoll = true;
    let failures = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let controller: AbortController | undefined;

    function schedule(): void {
      if (disposed || !shouldPoll || document.visibilityState === "hidden") return;
      if (timer !== undefined) clearTimeout(timer);
      timer = setTimeout(() => void poll(), 1000);
    }

    async function poll(): Promise<void> {
      if (disposed || document.visibilityState === "hidden") return;
      controller = new AbortController();
      try {
        const response = await getEvalRun({
          path: { id: runID },
          signal: controller.signal,
          throwOnError: true,
        });
        if (disposed || controller.signal.aborted) return;
        failures = 0;
        setError(null);
        setRun(response.data);
        shouldPoll = activeStatuses.has(response.data.status);
        if (shouldPoll) schedule();
      } catch (reason) {
        if (disposed || controller.signal.aborted) return;
        failures++;
        setError(errorMessage(reason));
        if (isTransient(reason) && failures < 3) schedule();
        else {
          shouldPoll = false;
          setStopped(true);
        }
      }
    }

    function handleVisibility(): void {
      if (document.visibilityState === "hidden") {
        if (timer !== undefined) clearTimeout(timer);
        controller?.abort();
      } else schedule();
    }

    setError(null);
    setStopped(false);
    document.addEventListener("visibilitychange", handleVisibility);
    void poll();
    return () => {
      disposed = true;
      if (timer !== undefined) clearTimeout(timer);
      controller?.abort();
      document.removeEventListener("visibilitychange", handleVisibility);
    };
  }, [retryGeneration, runID]);

  return { error, retry: () => setRetryGeneration((value) => value + 1), run, stopped };
}

function errorMessage(reason: unknown): string {
  return reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to refresh run";
}

function isTransient(reason: unknown): boolean {
  return !(reason instanceof Problem) || reason.status === 0 || reason.status >= 500;
}
