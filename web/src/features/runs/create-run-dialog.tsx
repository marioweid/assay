import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { Problem } from "@/api/errors";
import { createEvalRun } from "@/api/generated/sdk.gen";
import type { DatasetResponse } from "@/api/generated/types.gen";
import { Modal } from "@/components/modal";

type CreateRunDialogProps = {
  appID: string;
  datasets: DatasetResponse[];
  onClose: () => void;
};

export function CreateRunDialog({ appID, datasets, onClose }: CreateRunDialogProps) {
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
    <Modal label="New evaluation run" onClose={onClose}>
      <RunForm
        datasets={datasets}
        datasetID={datasetID}
        error={error}
        mode={mode}
        name={name}
        onClose={onClose}
        onDataset={setDatasetID}
        onMode={setMode}
        onName={setName}
        onSubmit={submit}
        onToggleScorer={toggleScorer}
        scorers={scorers}
        submitting={submitting}
      />
    </Modal>
  );
}

type RunFormProps = {
  datasetID: string;
  datasets: DatasetResponse[];
  error: string | null;
  mode: "score_existing" | "generate_then_score";
  name: string;
  scorers: string[];
  submitting: boolean;
  onClose: () => void;
  onDataset: (id: string) => void;
  onMode: (mode: "score_existing" | "generate_then_score") => void;
  onName: (name: string) => void;
  onSubmit: (event: React.FormEvent<HTMLFormElement>) => Promise<void>;
  onToggleScorer: (scorer: string) => void;
};

function RunForm(props: RunFormProps) {
  return (
    <form
      className="w-full max-w-lg border border-line bg-white p-6"
      onSubmit={(event) => void props.onSubmit(event)}
    >
      <h2 className="text-xl font-semibold">New evaluation run</h2>
      {props.error && (
        <p className="mt-4 border border-red-300 bg-red-50 p-3 text-sm" role="alert">
          {props.error}
        </p>
      )}
      <label className="mt-5 block text-sm font-medium">
        Run name
        <input
          autoFocus
          className="mt-1 block w-full border border-line px-3 py-2"
          onChange={(event) => props.onName(event.target.value)}
          value={props.name}
        />
      </label>
      <label className="mt-4 block text-sm font-medium">
        Dataset
        <select
          className="mt-1 block w-full border border-line px-3 py-2"
          onChange={(event) => props.onDataset(event.target.value)}
          value={props.datasetID}
        >
          <option value="">Select a dataset</option>
          {props.datasets.map((dataset) => (
            <option key={dataset.id} value={dataset.id}>
              {dataset.name}
            </option>
          ))}
        </select>
      </label>
      <label className="mt-4 block text-sm font-medium">
        Mode
        <select
          className="mt-1 block w-full border border-line px-3 py-2"
          onChange={(event) => props.onMode(event.target.value as RunFormProps["mode"])}
          value={props.mode}
        >
          <option value="score_existing">Score existing outputs</option>
          <option value="generate_then_score">Generate then score</option>
        </select>
      </label>
      <fieldset className="mt-4">
        <legend className="text-sm font-medium">Scorers</legend>
        {["groundedness", "correctness"].map((scorer) => (
          <label className="mr-5 mt-2 inline-flex items-center gap-2 text-sm" key={scorer}>
            <input
              checked={props.scorers.includes(scorer)}
              onChange={() => props.onToggleScorer(scorer)}
              type="checkbox"
            />
            {scorer}
          </label>
        ))}
      </fieldset>
      <div className="mt-6 flex justify-end gap-3">
        <button className="px-4 py-2 text-sm" onClick={props.onClose} type="button">
          Close
        </button>
        <button
          className="bg-blue-700 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
          disabled={props.submitting}
          type="submit"
        >
          Create run
        </button>
      </div>
    </form>
  );
}
