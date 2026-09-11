import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";

import { Problem } from "@/api/errors";
import {
  attachTraceReference,
  createDatasetItemFromTrace,
  getTraceScoringEligibility,
  listDatasets,
  scoreTraces,
} from "@/api/generated/sdk.gen";
import type {
  DatasetResponse,
  ScoringEligibilityResponse,
  ScoringTaskResponse,
  TraceResponse,
} from "@/api/generated/types.gen";
import { Dialog } from "@/components/ui/dialog";

type Scorer = "groundedness" | "correctness";

type TraceActionsProps = {
  appId: string;
  onChanged: () => Promise<void>;
  trace: TraceResponse;
};

export function TraceActions({ appId, onChanged, trace }: TraceActionsProps) {
  const [eligibility, setEligibility] = useState<ScoringEligibilityResponse[]>([]);
  const [eligibilityError, setEligibilityError] = useState<string | null>(null);
  const [scoring, setScoring] = useState(false);
  const [saving, setSaving] = useState(false);
  const [editing, setEditing] = useState(false);

  const refreshEligibility = async (): Promise<void> => {
    try {
      const response = await getTraceScoringEligibility({
        path: { id: trace.id },
        throwOnError: true,
      });
      setEligibility(response.data.items ?? []);
      setEligibilityError(null);
    } catch (reason) {
      setEligibilityError(errorMessage(reason, "Unable to load scoring eligibility"));
    }
  };

  useEffect(() => {
    void refreshEligibility();
  }, [trace.id]);

  const changed = async (): Promise<void> => {
    await onChanged();
    await refreshEligibility();
  };

  return (
    <section
      className="mt-4 border border-line bg-surface p-4"
      aria-labelledby="trace-actions-heading"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="font-semibold" id="trace-actions-heading">
            Actions
          </h2>
          <p className="text-sm text-muted">
            Queue a score, edit its reference, or save retained evidence.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            className="border border-line px-3 py-2 text-sm"
            onClick={() => setEditing(true)}
            type="button"
          >
            Edit reference
          </button>
          <button
            className="border border-line px-3 py-2 text-sm"
            onClick={() => setScoring(true)}
            type="button"
          >
            Score
          </button>
          <button
            className="border border-line px-3 py-2 text-sm"
            onClick={() => setSaving(true)}
            type="button"
          >
            Save to dataset
          </button>
        </div>
      </div>
      {eligibilityError !== null && (
        <p className="mt-3 text-sm text-danger" role="alert">
          {eligibilityError}
        </p>
      )}
      <Eligibility items={eligibility} />
      {editing && (
        <ReferenceEditor onChanged={changed} onClose={() => setEditing(false)} trace={trace} />
      )}
      {scoring && (
        <ScoreDialog
          eligibility={eligibility}
          onChanged={changed}
          onClose={() => setScoring(false)}
          traceID={trace.id}
        />
      )}
      {saving && (
        <SaveToDatasetDialog
          appId={appId}
          onChanged={changed}
          onClose={() => setSaving(false)}
          trace={trace}
        />
      )}
    </section>
  );
}

function Eligibility({ items }: { items: ScoringEligibilityResponse[] }) {
  if (items.length === 0) return null;
  return (
    <ul className="mt-3 space-y-1 text-sm">
      {items.map((item) => (
        <li key={item.scorer}>
          <strong>{item.scorer}</strong>:{" "}
          {item.eligible
            ? "eligible"
            : (item.reasons ?? []).map((reason) => reason.message).join(" ")}
        </li>
      ))}
    </ul>
  );
}

function ReferenceEditor({
  onChanged,
  onClose,
  trace,
}: {
  onChanged: () => Promise<void>;
  onClose: () => void;
  trace: TraceResponse;
}) {
  const [value, setValue] = useState(trace.reference_answer ?? "");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const save = async (): Promise<void> => {
    setPending(true);
    try {
      await attachTraceReference({
        path: { id: trace.id },
        body: { reference_answer: value },
        throwOnError: true,
      });
      await onChanged();
      onClose();
    } catch (reason) {
      setError(errorMessage(reason, "Unable to save reference"));
    } finally {
      setPending(false);
    }
  };
  return (
    <div className="mt-3 border-t border-line pt-3">
      <label className="block text-sm font-medium" htmlFor="trace-reference">
        Reference answer
      </label>
      <textarea
        className="mt-1 w-full border border-line bg-canvas p-2"
        id="trace-reference"
        onChange={(event) => setValue(event.target.value)}
        value={value}
      />
      {error !== null && (
        <p className="mt-2 text-sm text-danger" role="alert">
          {error}
        </p>
      )}
      <div className="mt-2 flex gap-2">
        <button
          className="border border-line px-3 py-2 text-sm"
          disabled={pending}
          onClick={() => void save()}
          type="button"
        >
          Save reference
        </button>
        <button className="px-3 py-2 text-sm" onClick={onClose} type="button">
          Cancel
        </button>
      </div>
    </div>
  );
}

function ScoreDialog({
  eligibility,
  onChanged,
  onClose,
  traceID,
}: {
  eligibility: ScoringEligibilityResponse[];
  onChanged: () => Promise<void>;
  onClose: () => void;
  traceID: string;
}) {
  const eligible = eligibility.filter((item) => item.eligible).map((item) => item.scorer);
  const [selected, setSelected] = useState<string[]>(eligible);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [tasks, setTasks] = useState<ScoringTaskResponse[]>([]);
  const refresh = async (): Promise<void> => {
    try {
      await onChanged();
      setError(null);
    } catch (reason) {
      setError(
        `Scoring was queued, but the trace could not refresh: ${errorMessage(reason, "Unable to refresh trace")}`,
      );
    }
  };
  const score = async (): Promise<void> => {
    setPending(true);
    try {
      const response = await scoreTraces({
        body: { trace_ids: [traceID], scorers: selected },
        throwOnError: true,
      });
      setTasks(response.data.items ?? []);
    } catch (reason) {
      setError(errorMessage(reason, "Unable to queue scoring"));
      setPending(false);
      return;
    }
    await refresh();
    setPending(false);
  };
  const refreshTasks = async (): Promise<void> => {
    setPending(true);
    await refresh();
    setPending(false);
  };
  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open title="Score trace">
      <fieldset>
        <legend className="text-sm text-muted">Only eligible scorers can be queued.</legend>
        {eligibility.map((item) => (
          <label className="mt-3 flex gap-2" key={item.scorer}>
            <input
              checked={selected.includes(item.scorer)}
              disabled={!item.eligible}
              onChange={() =>
                setSelected((current) =>
                  current.includes(item.scorer)
                    ? current.filter((scorer) => scorer !== item.scorer)
                    : [...current, item.scorer],
                )
              }
              type="checkbox"
            />
            {item.scorer}
            {!item.eligible &&
              ` — ${(item.reasons ?? []).map((reason) => reason.message).join(" ")}`}
          </label>
        ))}
      </fieldset>
      {error !== null && (
        <p className="mt-3 text-sm text-danger" role="alert">
          {error}
        </p>
      )}
      {tasks.length > 0 && (
        <div className="mt-3" role="status">
          <p className="font-medium">Queued scoring tasks</p>
          <ul className="mt-1 text-sm text-muted">
            {tasks.map((task) => (
              <li key={task.id}>
                {task.scorer} — {task.status}
              </li>
            ))}
          </ul>
        </div>
      )}
      <div className="mt-4 flex gap-2">
        {tasks.length === 0 && (
          <button
            className="border border-line px-3 py-2 text-sm"
            disabled={pending || selected.length === 0}
            onClick={() => void score()}
            type="button"
          >
            Queue score
          </button>
        )}
        {tasks.length > 0 && (
          <button
            className="border border-line px-3 py-2 text-sm"
            disabled={pending}
            onClick={() => void refreshTasks()}
            type="button"
          >
            Refresh trace
          </button>
        )}
        <button className="px-3 py-2 text-sm" onClick={onClose} type="button">
          {tasks.length > 0 ? "Close" : "Cancel"}
        </button>
      </div>
    </Dialog>
  );
}

function SaveToDatasetDialog({
  appId,
  onChanged,
  onClose,
  trace,
}: {
  appId: string;
  onChanged: () => Promise<void>;
  onClose: () => void;
  trace: TraceResponse;
}) {
  const choices = useMemo(() => latestScoreChoices(trace), [trace.scores]);
  const [datasets, setDatasets] = useState<DatasetResponse[]>([]);
  const [datasetID, setDatasetID] = useState("");
  const [scorer, setScorer] = useState<Scorer | "">(
    (choices[0]?.scorer as Scorer | undefined) ?? "",
  );
  const [expectedOutput, setExpectedOutput] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [savedDatasetID, setSavedDatasetID] = useState<string | null>(null);
  useEffect(() => {
    let cancelled = false;
    const load = async (): Promise<void> => {
      try {
        const items = await loadDatasets(appId);
        if (cancelled) return;
        setDatasets(items);
        setDatasetID(items[0]?.id ?? "");
      } catch (reason) {
        if (!cancelled) setError(errorMessage(reason, "Unable to load datasets"));
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [appId]);
  const save = async (): Promise<void> => {
    setPending(true);
    try {
      const response = await createDatasetItemFromTrace({
        path: { id: datasetID },
        body: {
          trace_id: trace.id,
          scorer: scorer as Scorer,
          ...(expectedOutput.trim() === "" ? {} : { expected_output: expectedOutput }),
        },
        throwOnError: true,
      });
      await onChanged();
      setError(null);
      setSavedDatasetID(response.data.dataset_id);
    } catch (reason) {
      setError(errorMessage(reason, "Unable to save score evidence"));
    } finally {
      setPending(false);
    }
  };
  return (
    <Dialog onOpenChange={(open) => !open && onClose()} open title="Save to dataset">
      <label className="block text-sm">
        Dataset
        <select
          className="mt-1 block w-full border border-line bg-canvas p-2"
          onChange={(event) => setDatasetID(event.target.value)}
          value={datasetID}
        >
          <option value="">Select dataset</option>
          {datasets.map((dataset) => (
            <option key={dataset.id} value={dataset.id}>
              {dataset.name}
            </option>
          ))}
        </select>
      </label>
      <label className="mt-3 block text-sm">
        Latest score
        <select
          className="mt-1 block w-full border border-line bg-canvas p-2"
          onChange={(event) => setScorer(event.target.value as Scorer)}
          value={scorer}
        >
          {choices.map((choice) => (
            <option key={choice.scorer} value={choice.scorer}>
              {choice.scorer} ({choice.value})
            </option>
          ))}
        </select>
      </label>
      <label className="mt-3 block text-sm">
        Reference override (optional)
        <textarea
          className="mt-1 block w-full border border-line bg-canvas p-2"
          onChange={(event) => setExpectedOutput(event.target.value)}
          value={expectedOutput}
        />
      </label>
      {error !== null && (
        <p className="mt-3 text-sm text-danger" role="alert">
          {error}
        </p>
      )}
      {savedDatasetID !== null && (
        <Link
          className="mt-3 block text-accent hover:underline"
          to={`/apps/${appId}/datasets/${savedDatasetID}`}
        >
          Open dataset
        </Link>
      )}
      <div className="mt-4 flex gap-2">
        <button
          className="border border-line px-3 py-2 text-sm"
          disabled={pending || datasetID === "" || scorer === ""}
          onClick={() => void save()}
          type="button"
        >
          Save evidence
        </button>
        <button className="px-3 py-2 text-sm" onClick={onClose} type="button">
          Cancel
        </button>
      </div>
    </Dialog>
  );
}

async function loadDatasets(appId: string): Promise<DatasetResponse[]> {
  const items: DatasetResponse[] = [];
  const cursors = new Set<string>();
  let cursor: string | undefined;
  do {
    const response = await listDatasets({
      query: {
        application_id: appId,
        limit: 500,
        ...(cursor === undefined ? {} : { cursor }),
      },
      throwOnError: true,
    });
    items.push(...(response.data.items ?? []));
    cursor = response.data.next_cursor;
    if (cursor !== undefined && cursors.has(cursor)) {
      throw new Error("The server returned a repeated dataset cursor");
    }
    if (cursor !== undefined) cursors.add(cursor);
  } while (cursor !== undefined);
  return items;
}

function latestScoreChoices(trace: TraceResponse) {
  const scores = trace.scores ?? [];
  return [...scores]
    .sort(
      (left, right) =>
        Date.parse(right.created_at) - Date.parse(left.created_at) || right.id - left.id,
    )
    .filter(
      (score, index, sorted) => sorted.findIndex((item) => item.scorer === score.scorer) === index,
    );
}

function errorMessage(reason: unknown, fallback: string): string {
  return reason instanceof Problem ? (reason.detail ?? reason.title) : fallback;
}
