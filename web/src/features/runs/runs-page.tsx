import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router";

import { Problem } from "@/api/errors";
import { listDatasets, listEvalRuns } from "@/api/generated/sdk.gen";
import type { DatasetResponse, EvalRunResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { fieldControlClass } from "@/components/ui/field";
import { CreateRunDialog } from "@/features/runs/create-run-dialog";
import { isActiveRun } from "@/features/runs/run-status";

export function RunsPage() {
  const { appId = "" } = useParams();
  const requestNumber = useRef(0);
  const [runs, setRuns] = useState<EvalRunResponse[]>([]);
  const [datasets, setDatasets] = useState<DatasetResponse[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [loadGeneration, setLoadGeneration] = useState(0);
  const [nextDatasetCursor, setNextDatasetCursor] = useState<string | null>(null);
  const [loadingDatasets, setLoadingDatasets] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const currentRequest = ++requestNumber.current;
    setRuns([]);
    setDatasets([]);
    setLoading(true);
    setError(null);
    void Promise.all([
      listEvalRuns({
        query: { application_id: appId },
        signal: controller.signal,
        throwOnError: true,
      }),
      listDatasets({
        query: { application_id: appId },
        signal: controller.signal,
        throwOnError: true,
      }),
    ])
      .then(([runResponse, datasetResponse]) => {
        if (controller.signal.aborted || requestNumber.current !== currentRequest) return;
        setRuns(runResponse.data.items ?? []);
        setDatasets(datasetResponse.data.items ?? []);
        setNextDatasetCursor(datasetResponse.data.next_cursor ?? null);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted && requestNumber.current === currentRequest) {
          setError(
            reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to load runs",
          );
        }
      })
      .finally(() => {
        if (requestNumber.current === currentRequest) setLoading(false);
      });
    return () => controller.abort();
  }, [appId, loadGeneration]);

  async function loadMoreDatasets(): Promise<void> {
    if (nextDatasetCursor === null || loadingDatasets) return;
    setLoadingDatasets(true);
    try {
      const response = await listDatasets({
        query: { application_id: appId, cursor: nextDatasetCursor },
        throwOnError: true,
      });
      setDatasets((current) => {
        const ids = new Set(current.map((dataset) => dataset.id));
        return [
          ...current,
          ...(response.data.items ?? []).filter((dataset) => !ids.has(dataset.id)),
        ];
      });
      setNextDatasetCursor(response.data.next_cursor ?? null);
    } catch (reason) {
      setError(
        reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to load datasets",
      );
    } finally {
      setLoadingDatasets(false);
    }
  }

  return (
    <section aria-labelledby="runs-heading">
      <div className="flex items-end justify-between gap-4">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.14em] text-muted">
            Offline evaluation
          </p>
          <h1 className="mt-1 text-2xl font-semibold" id="runs-heading">
            Evaluation runs
          </h1>
        </div>
        <button
          className="bg-blue-700 px-4 py-2 text-sm font-medium text-white"
          onClick={() => setDialogOpen(true)}
        >
          New evaluation run
        </button>
      </div>
      {error && (
        <p className="mt-4 border border-red-300 bg-red-50 p-3 text-sm" role="alert">
          {error}
        </p>
      )}
      {loading && <p className="mt-8 text-muted">Loading runs...</p>}
      {!loading && runs.length > 0 && <RunComparisonForm appID={appId} runs={runs} />}
      {!loading && error === null && runs.length === 0 && (
        <p className="mt-8 text-muted">No evaluation runs yet. Start one from a dataset.</p>
      )}
      {runs.length > 0 && <RunsTable appID={appId} datasets={datasets} runs={runs} />}
      {dialogOpen && (
        <CreateRunDialog
          appID={appId}
          datasets={datasets}
          loadingDatasets={loadingDatasets}
          nextDatasetCursor={nextDatasetCursor}
          onClose={() => setDialogOpen(false)}
          onLoadMoreDatasets={loadMoreDatasets}
          onUncertainOutcome={() => {
            setDialogOpen(false);
            setLoadGeneration((value) => value + 1);
          }}
        />
      )}
    </section>
  );
}

function RunComparisonForm({ appID, runs }: { appID: string; runs: EvalRunResponse[] }) {
  const navigate = useNavigate();
  const terminal = runs.filter((run) => !isActiveRun(run.status));
  const [baselineID, setBaselineID] = useState("");
  const [candidateID, setCandidateID] = useState("");
  const [scorer, setScorer] = useState("");
  const baseline = terminal.find((run) => run.id === baselineID);
  const candidate = terminal.find((run) => run.id === candidateID);
  const sharedScorers = (baseline?.scorers ?? []).filter((value) =>
    (candidate?.scorers ?? []).includes(value),
  );

  function selectPair(setter: (value: string) => void, value: string): void {
    setter(value);
    setScorer("");
  }

  function compare(event: React.FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    if (baselineID === "" || candidateID === "" || baselineID === candidateID || scorer === "")
      return;
    const query = new URLSearchParams({ baseline: baselineID, candidate: candidateID, scorer });
    navigate(`/apps/${appID}/runs/compare?${query.toString()}`);
  }

  if (terminal.length < 2) {
    return <p className="mt-6 text-sm text-muted">Complete two runs to compare matched cases.</p>;
  }
  return (
    <form className="mt-6 border border-line bg-surface p-4" onSubmit={compare}>
      <h2 className="font-semibold">Compare terminal runs</h2>
      <div className="mt-3 grid gap-3 md:grid-cols-4">
        <RunSelect
          label="Baseline run"
          onChange={(value) => selectPair(setBaselineID, value)}
          runs={terminal}
          value={baselineID}
        />
        <RunSelect
          label="Candidate run"
          onChange={(value) => selectPair(setCandidateID, value)}
          runs={terminal}
          value={candidateID}
        />
        <label className="text-sm">
          <span className="font-medium">Shared scorer</span>
          <select
            className={fieldControlClass + " mt-1"}
            disabled={baseline === undefined || candidate === undefined}
            onChange={(event) => setScorer(event.target.value)}
            value={sharedScorers.includes(scorer) ? scorer : ""}
          >
            <option value="">Select a scorer</option>
            {sharedScorers.map((value) => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </select>
        </label>
        <div className="flex items-end">
          <Button
            disabled={
              baselineID === "" || candidateID === "" || baselineID === candidateID || scorer === ""
            }
            type="submit"
            variant="primary"
          >
            Compare runs
          </Button>
        </div>
      </div>
    </form>
  );
}

function RunSelect({
  label,
  onChange,
  runs,
  value,
}: {
  label: string;
  onChange: (value: string) => void;
  runs: EvalRunResponse[];
  value: string;
}) {
  return (
    <label className="text-sm">
      <span className="font-medium">{label}</span>
      <select
        className={fieldControlClass + " mt-1"}
        onChange={(event) => onChange(event.target.value)}
        value={value}
      >
        <option value="">Select a run</option>
        {runs.map((run) => (
          <option key={run.id} value={run.id}>
            {run.name}
          </option>
        ))}
      </select>
    </label>
  );
}

function RunsTable({
  appID,
  datasets,
  runs,
}: {
  appID: string;
  datasets: DatasetResponse[];
  runs: EvalRunResponse[];
}) {
  const names = new Map(datasets.map((dataset) => [dataset.id, dataset.name]));
  return (
    <div className="mt-6 overflow-x-auto border border-line bg-surface">
      <table className="w-full text-left text-sm">
        <thead className="border-b border-line bg-canvas text-xs uppercase tracking-wide text-muted">
          <tr>
            {["Name", "Dataset", "Mode", "Status", "Progress", "Aggregates"].map((heading) => (
              <th className="px-4 py-3" key={heading}>
                {heading}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {runs.map((run) => (
            <tr className="border-b border-line last:border-0" key={run.id}>
              <td className="px-4 py-3">
                <Link
                  className="font-medium text-accent hover:underline"
                  to={`/apps/${appID}/runs/${run.id}`}
                >
                  {run.name}
                </Link>
              </td>
              <td className="px-4 py-3">{names.get(run.dataset_id) ?? run.dataset_id}</td>
              <td className="px-4 py-3">{modeLabel(run.mode)}</td>
              <td className="px-4 py-3 capitalize">{run.status}</td>
              <td className="px-4 py-3">
                {run.succeeded_items} / {run.total_items}
              </td>
              <td className="px-4 py-3">{aggregateSummary(run)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function modeLabel(mode: string): string {
  return mode === "generate_then_score" ? "Generate then score" : "Score existing";
}
function aggregateSummary(run: EvalRunResponse): string {
  const summaries = Object.entries(run.aggregates).map(
    ([scorer, value]) => `${scorer} ${value.mean.toFixed(2)}`,
  );
  return summaries.length === 0 ? "Pending" : summaries.join(", ");
}
