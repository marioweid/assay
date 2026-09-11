import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { Problem } from "@/api/errors";
import { createEvalRun } from "@/api/generated/sdk.gen";
import type { DatasetResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { fieldControlClass } from "@/components/ui/field";

type CreateRunDialogProps = {
  appID: string;
  datasets: DatasetResponse[];
  loadingDatasets: boolean;
  nextDatasetCursor: string | null;
  onClose: () => void;
  onLoadMoreDatasets: () => Promise<void>;
};

export function CreateRunDialog({
  appID,
  datasets,
  loadingDatasets,
  nextDatasetCursor,
  onClose,
  onLoadMoreDatasets,
}: CreateRunDialogProps) {
  const navigate = useNavigate();
  const activeRequest = useRef<AbortController | null>(null);
  const [name, setName] = useState("");
  const [datasetID, setDatasetID] = useState("");
  const [mode, setMode] = useState<"score_existing" | "generate_then_score">("score_existing");
  const [scorers, setScorers] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => () => activeRequest.current?.abort(), []);

  function toggleScorer(scorer: string): void {
    setScorers((current) =>
      current.includes(scorer) ? current.filter((item) => item !== scorer) : [...current, scorer],
    );
  }

  async function submit(event: React.FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (name.trim() === "" || datasetID === "" || scorers.length === 0) {
      setError("Run name, dataset, and scorer are required.");
      return;
    }
    setSubmitting(true);
    setError(null);
    const controller = new AbortController();
    activeRequest.current = controller;
    try {
      const response = await createEvalRun({
        body: { application_id: appID, dataset_id: datasetID, mode, name: name.trim(), scorers },
        signal: controller.signal,
        throwOnError: true,
      });
      navigate(`/apps/${appID}/runs/${response.data.id}`);
    } catch (reason) {
      if (!controller.signal.aborted)
        setError(
          reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to create run",
        );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      open
      title="New evaluation run"
    >
      <form className="mt-4 space-y-4" onSubmit={(event) => void submit(event)}>
        {error !== null && (
          <p className="border border-danger/30 bg-danger/10 p-3 text-sm text-danger" role="alert">
            {error}
          </p>
        )}
        <label className="block text-sm">
          <span className="font-medium text-ink">Run name</span>
          <input
            autoFocus
            className={fieldControlClass + " mt-1"}
            onChange={(event) => setName(event.target.value)}
            value={name}
          />
        </label>
        <label className="block text-sm">
          <span className="font-medium text-ink">Dataset</span>
          <select
            className={fieldControlClass + " mt-1"}
            onChange={(event) => setDatasetID(event.target.value)}
            value={datasetID}
          >
            <option value="">Select a dataset</option>
            {datasets.map((dataset) => (
              <option key={dataset.id} value={dataset.id}>
                {dataset.name}
              </option>
            ))}
          </select>
        </label>
        {nextDatasetCursor !== null && (
          <Button disabled={loadingDatasets} onClick={() => void onLoadMoreDatasets()}>
            {loadingDatasets ? "Loading datasets..." : "Load more datasets"}
          </Button>
        )}
        <label className="block text-sm">
          <span className="font-medium text-ink">Mode</span>
          <select
            className={fieldControlClass + " mt-1"}
            onChange={(event) =>
              setMode(event.target.value as "score_existing" | "generate_then_score")
            }
            value={mode}
          >
            <option value="score_existing">Score existing outputs</option>
            <option value="generate_then_score">Generate then score</option>
          </select>
        </label>
        <fieldset className="space-y-1">
          <legend className="text-sm font-medium text-ink">Scorers</legend>
          <p className="text-sm text-muted">
            {mode === "score_existing"
              ? "Every item needs a recorded answer. If your dataset only has questions and expected answers, select Generate then score."
              : "Calls the application's configured target endpoint to generate an answer for each question."}
          </p>
          {["groundedness", "correctness"].map((scorer) => (
            <label className="mt-2 inline-flex items-center gap-2 text-sm" key={scorer}>
              <input
                checked={scorers.includes(scorer)}
                onChange={() => toggleScorer(scorer)}
                type="checkbox"
              />
              {scorer}
            </label>
          ))}
        </fieldset>
        <div className="flex justify-end gap-3">
          <Button onClick={onClose}>Close</Button>
          <Button disabled={submitting} type="submit" variant="primary">
            Create run
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
