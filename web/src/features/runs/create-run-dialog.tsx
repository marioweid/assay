import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { Problem } from "@/api/errors";
import { createEvalRun } from "@/api/generated/sdk.gen";
import type { DatasetResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { fieldControlClass } from "@/components/ui/field";

type RunDefaults = {
  datasetID: string;
  mode: "score_existing" | "generate_then_score";
  name: string;
  scorers: string[];
};

type CreateRunDialogProps = {
  appID: string;
  initial?: RunDefaults;
  datasets: DatasetResponse[];
  loadingDatasets: boolean;
  nextDatasetCursor: string | null;
  onClose: () => void;
  onLoadMoreDatasets: () => Promise<void>;
  onUncertainOutcome: () => void;
};

export function CreateRunDialog({
  appID,
  datasets,
  initial,
  loadingDatasets,
  nextDatasetCursor,
  onClose,
  onLoadMoreDatasets,
  onUncertainOutcome,
}: CreateRunDialogProps) {
  const navigate = useNavigate();
  const activeRequest = useRef<AbortController | null>(null);
  const [name, setName] = useState(initial?.name ?? "");
  const [datasetID, setDatasetID] = useState(initial?.datasetID ?? "");
  const [mode, setMode] = useState<"score_existing" | "generate_then_score">(
    initial?.mode ?? "score_existing",
  );
  const [scorers, setScorers] = useState<string[]>(initial?.scorers ?? []);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [uncertain, setUncertain] = useState(false);

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
      if (!controller.signal.aborted) {
        if (!(reason instanceof Problem) || reason.status === 0) {
          setUncertain(true);
          setError(
            "Creation outcome is unknown. Close this dialog and refresh the run list before trying again.",
          );
        } else {
          setError(reason.detail ?? reason.title);
        }
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog
      onOpenChange={(open) => {
        if (!open && !submitting && !uncertain) onClose();
      }}
      open
      title={initial === undefined ? "New evaluation run" : "Run evaluation again"}
    >
      <form className="mt-4 space-y-4" onSubmit={(event) => void submit(event)}>
        {initial !== undefined && (
          <p className="text-sm text-muted">
            Uses the current dataset and current application/scorer configuration. This does not
            replay the old snapshot or retry only failed cases.
          </p>
        )}
        {error !== null && (
          <div
            className="border border-danger/30 bg-danger/10 p-3 text-sm text-danger"
            role="alert"
          >
            <p>{error}</p>
            {uncertain && (
              <Button className="mt-3" onClick={onUncertainOutcome}>
                Refresh run list
              </Button>
            )}
          </div>
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
            {initial !== undefined &&
              !datasets.some((dataset) => dataset.id === initial.datasetID) && (
                <option value={initial.datasetID}>Current dataset ({initial.datasetID})</option>
              )}
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
          <Button disabled={submitting || uncertain} onClick={onClose}>
            Close
          </Button>
          <Button disabled={submitting || uncertain} type="submit" variant="primary">
            Create run
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
