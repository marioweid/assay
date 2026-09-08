import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";

import { Problem } from "@/api/errors";
import { listDatasets } from "@/api/generated/sdk.gen";
import type { DatasetResponse } from "@/api/generated/types.gen";
import { CreateDatasetDialog } from "@/features/datasets/create-dataset-dialog";

export function DatasetsPage() {
  const { appId = "" } = useParams();
  const requestNumber = useRef(0);
  const [datasets, setDatasets] = useState<DatasetResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    const currentRequest = ++requestNumber.current;
    setDatasets([]);
    setLoading(true);
    setError(null);
    setCreating(false);
    void listDatasets({
      query: { application_id: appId },
      signal: controller.signal,
      throwOnError: true,
    })
      .then((response) => {
        if (!controller.signal.aborted && requestNumber.current === currentRequest) {
          setDatasets(response.data.items ?? []);
        }
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted && requestNumber.current === currentRequest) {
          setError(
            reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to load datasets",
          );
        }
      })
      .finally(() => {
        if (requestNumber.current === currentRequest) setLoading(false);
      });
    return () => controller.abort();
  }, [appId]);

  return (
    <section aria-labelledby="datasets-heading">
      <p className="font-mono text-xs uppercase tracking-[0.14em] text-muted">Evaluation corpus</p>
      <h1 className="mt-1 text-2xl font-semibold" id="datasets-heading">
        Datasets
      </h1>
      <button
        className="mt-4 bg-blue-700 px-4 py-2 text-sm text-white"
        onClick={() => setCreating(true)}
      >
        Create dataset
      </button>
      {creating && (
        <CreateDatasetDialog key={appId} appID={appId} onClose={() => setCreating(false)} />
      )}
      {error && (
        <p className="mt-4 border border-red-300 bg-red-50 p-3 text-sm" role="alert">
          {error}
        </p>
      )}
      {loading && <p className="mt-8 text-muted">Loading datasets...</p>}
      {!loading && error === null && datasets.length === 0 && (
        <p className="mt-8 text-muted">No datasets yet. Create one to collect evaluation cases.</p>
      )}
      {datasets.length > 0 && (
        <div className="mt-6 overflow-x-auto border border-line bg-white">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-line bg-slate-50 text-xs uppercase tracking-wide text-muted">
              <tr>
                <th className="px-4 py-3">Name</th>
                <th className="px-4 py-3">Description</th>
                <th className="px-4 py-3">Updated</th>
              </tr>
            </thead>
            <tbody>
              {datasets.map((dataset) => (
                <tr className="border-b border-line last:border-0" key={dataset.id}>
                  <td className="px-4 py-3">
                    <Link
                      className="font-medium text-blue-700 hover:underline"
                      to={`/apps/${appId}/datasets/${dataset.id}`}
                    >
                      {dataset.name}
                    </Link>
                  </td>
                  <td className="px-4 py-3 text-muted">
                    {dataset.description?.trim() || "No description"}
                  </td>
                  <td className="whitespace-nowrap px-4 py-3">
                    {new Date(dataset.updated_at).toLocaleString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
