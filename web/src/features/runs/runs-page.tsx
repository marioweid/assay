import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";

import { Problem } from "@/api/errors";
import { listDatasets, listEvalRuns } from "@/api/generated/sdk.gen";
import type { DatasetResponse, EvalRunResponse } from "@/api/generated/types.gen";
import { CreateRunDialog } from "@/features/runs/create-run-dialog";

export function RunsPage() {
  const { appId = "" } = useParams();
  const requestNumber = useRef(0);
  const [runs, setRuns] = useState<EvalRunResponse[]>([]);
  const [datasets, setDatasets] = useState<DatasetResponse[]>([]);
  const [dialogOpen, setDialogOpen] = useState(false);
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
  }, [appId]);

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
      {!loading && error === null && runs.length === 0 && (
        <p className="mt-8 text-muted">No evaluation runs yet. Start one from a dataset.</p>
      )}
      {runs.length > 0 && <RunsTable appID={appId} datasets={datasets} runs={runs} />}
      {dialogOpen && (
        <CreateRunDialog appID={appId} datasets={datasets} onClose={() => setDialogOpen(false)} />
      )}
    </section>
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
    <div className="mt-6 overflow-x-auto border border-line bg-white">
      <table className="w-full text-left text-sm">
        <thead className="border-b border-line bg-slate-50 text-xs uppercase tracking-wide text-muted">
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
                  className="font-medium text-blue-700 hover:underline"
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
