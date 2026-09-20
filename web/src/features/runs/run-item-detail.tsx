import { useEffect, useState } from "react";

import { Problem } from "@/api/errors";
import { getEvalRunItem } from "@/api/generated/sdk.gen";
import type { EvalRunItemResponse } from "@/api/generated/types.gen";
import { JsonView } from "@/components/json-view";
import { ScoreResult } from "@/components/score-result";
import { Button } from "@/components/ui/button";

export function RunItemDetail({
  itemID,
  onClose,
  runID,
}: {
  itemID: string;
  onClose: () => void;
  runID: string;
}) {
  const [item, setItem] = useState<EvalRunItemResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    setItem(null);
    setError(null);
    void getEvalRunItem({
      path: { id: runID, itemId: itemID },
      signal: controller.signal,
      throwOnError: true,
    })
      .then((response) => setItem(response.data))
      .catch((reason: unknown) => {
        if (!controller.signal.aborted)
          setError(
            reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to load case",
          );
      });
    return () => controller.abort();
  }, [itemID, runID]);

  if (error)
    return (
      <p className="mt-4 text-sm text-danger" role="alert">
        {error}
      </p>
    );
  if (item === null) return <p className="mt-4 text-sm text-muted">Loading case detail...</p>;

  const label = item.snapshot.external_id ?? item.dataset_item_id;
  return (
    <section aria-labelledby="run-case-detail-heading" className="mt-6 border border-line p-4">
      <div className="flex items-start justify-between gap-4">
        <h3 className="text-lg font-semibold" id="run-case-detail-heading">
          Case {label}
        </h3>
        <Button onClick={onClose}>Close detail</Button>
      </div>
      {item.error && <p className="mt-4 text-sm text-danger">Execution error: {item.error}</p>}
      <dl className="mt-4 grid gap-4 lg:grid-cols-2">
        <Evidence label="Original input" value={item.snapshot.input} />
        <Evidence label="Original context" value={item.snapshot.context} />
        <Evidence label="Recorded output" value={item.snapshot.output ?? null} />
        <Evidence label="Generated output" value={item.generated_output ?? null} />
        <Evidence label="Expected output" value={item.snapshot.expected_output ?? null} />
      </dl>
      <div className="mt-6 space-y-3">
        {item.scores.length === 0 ? (
          <p className="text-sm text-muted">No quality score was recorded.</p>
        ) : (
          item.scores.map((score) => <ScoreResult key={score.id} score={score} />)
        )}
      </div>
    </section>
  );
}

function Evidence({ label, value }: { label: string; value: unknown }) {
  return (
    <div>
      <dt className="text-sm font-medium">{label}</dt>
      <dd className="mt-2">
        <JsonView value={value} />
      </dd>
    </div>
  );
}
