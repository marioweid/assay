import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router";

import { Problem } from "@/api/errors";
import { listEvalRunItems } from "@/api/generated/sdk.gen";
import type { EvalRunItemResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/ui/status-badge";
import { RunItemDetail } from "@/features/runs/run-item-detail";

export function RunItems({ runID }: { runID: string }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedItemID = searchParams.get("item");
  const request = useRef<AbortController | null>(null);
  const [items, setItems] = useState<EvalRunItemResponse[]>([]);
  const [cursor, setCursor] = useState<string>();
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  async function load(nextCursor?: string): Promise<void> {
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError(null);
    try {
      const response = await listEvalRunItems({
        path: { id: runID },
        query: { limit: 100, ...(nextCursor === undefined ? {} : { cursor: nextCursor }) },
        signal: controller.signal,
        throwOnError: true,
      });
      if (controller.signal.aborted) return;
      const received = response.data.items ?? [];
      setItems((current) => (nextCursor === undefined ? received : [...current, ...received]));
      setCursor(response.data.next_cursor);
    } catch (reason) {
      if (!controller.signal.aborted)
        setError(
          reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to load cases",
        );
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }

  useEffect(() => {
    void load();
    return () => request.current?.abort();
  }, [runID]); // oxlint-disable-line react-hooks/exhaustive-deps -- load resets for this run ID

  return (
    <section aria-labelledby="run-cases-heading" className="mt-6">
      <h2 className="text-lg font-semibold" id="run-cases-heading">
        Cases
      </h2>
      {error && (
        <p className="mt-3 text-sm text-danger" role="alert">
          {error}
        </p>
      )}
      {!loading && items.length === 0 ? (
        <p className="mt-3 text-sm text-muted">No case results yet.</p>
      ) : (
        <div className="mt-3 overflow-x-auto border border-line bg-surface">
          <table className="w-full text-left text-sm">
            <thead className="bg-canvas">
              <tr>
                <th className="px-4 py-3">Case</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Scores</th>
              </tr>
            </thead>
            <tbody>
              {items.map((item) => (
                <tr className="border-t border-line" key={item.dataset_item_id}>
                  <td className="px-4 py-3">
                    <button
                      className="text-left text-accent hover:underline"
                      onClick={() => setSearchParams({ item: item.dataset_item_id })}
                    >
                      {item.snapshot.external_id ??
                        String(item.snapshot.input["question"] ?? item.dataset_item_id)}
                    </button>
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge
                      tone={
                        item.status === "succeeded"
                          ? "success"
                          : item.status === "failed"
                            ? "danger"
                            : "neutral"
                      }
                    >
                      {item.status}
                    </StatusBadge>
                    {item.error && <p className="mt-1 text-xs text-danger">{item.error}</p>}
                  </td>
                  <td className="px-4 py-3">
                    {item.scores.length === 0 ? (
                      <span className="text-muted">Not scored</span>
                    ) : (
                      item.scores.map((score) => (
                        <span className="mr-3 whitespace-nowrap" key={score.id}>
                          {score.scorer} {score.value.toFixed(2)} · {score.passed ? "Pass" : "Fail"}
                        </span>
                      ))
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {loading && <p className="mt-3 text-sm text-muted">Loading cases...</p>}
      {cursor && !loading && (
        <Button className="mt-3" onClick={() => void load(cursor)}>
          Load more cases
        </Button>
      )}
      {selectedItemID && (
        <RunItemDetail itemID={selectedItemID} onClose={() => setSearchParams({})} runID={runID} />
      )}
    </section>
  );
}
