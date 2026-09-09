import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";

import { Problem } from "@/api/errors";
import { cancelEvalRun } from "@/api/generated/sdk.gen";
import type { EvalRunResponse } from "@/api/generated/types.gen";
import { useRunPolling } from "@/features/runs/use-run-polling";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { isActiveRun } from "@/features/runs/run-status";

export function RunDetail() {
  const { appId = "", runId = "" } = useParams();
  const polling = useRunPolling(runId);
  const cancelRequest = useRef<AbortController | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [cancelError, setCancelError] = useState<string | null>(null);
  const [canceling, setCanceling] = useState(false);

  useEffect(() => () => cancelRequest.current?.abort(), []);

  async function cancel(): Promise<void> {
    setCanceling(true);
    setCancelError(null);
    const controller = new AbortController();
    cancelRequest.current = controller;
    try {
      await cancelEvalRun({ path: { id: runId }, signal: controller.signal, throwOnError: true });
      setConfirming(false);
      polling.retry();
    } catch (reason) {
      if (!controller.signal.aborted)
        setCancelError(
          reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to cancel run",
        );
    } finally {
      setCanceling(false);
    }
  }

  if (polling.run === null)
    return (
      <div>
        {polling.error && (
          <p className="border border-danger bg-danger/10 p-4" role="alert">
            {polling.error}
          </p>
        )}
        {!polling.stopped && <p className="text-muted">Loading evaluation run...</p>}
        {polling.stopped && (
          <button className="mt-3 border border-line px-4 py-2" onClick={polling.retry}>
            Retry polling
          </button>
        )}
      </div>
    );
  if (polling.run.application_id !== appId)
    return (
      <p className="border border-danger bg-danger/10 p-4" role="alert">
        Run does not belong to this application
      </p>
    );
  const run = polling.run;
  return (
    <section aria-labelledby="run-heading">
      <Link className="text-sm text-accent hover:underline" to={`/apps/${appId}/runs`}>
        Back to runs
      </Link>
      <div className="mt-4 flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="font-mono text-xs text-muted">{run.id}</p>
          <h1 className="mt-1 text-2xl font-semibold" id="run-heading">
            {run.name}
          </h1>
          <p className="mt-2 capitalize text-muted">{run.status}</p>
        </div>
        {isActiveRun(run.status) && (
          <button
            className="border border-danger px-4 py-2 text-sm text-danger"
            onClick={() => setConfirming(true)}
          >
            Cancel run
          </button>
        )}
      </div>
      {polling.error && (
        <p className="mt-4 border border-warning bg-warning/10 p-3 text-sm" role="alert">
          {polling.error}
          {polling.stopped && (
            <button className="ml-3 underline" onClick={polling.retry}>
              Retry polling
            </button>
          )}
        </p>
      )}
      {cancelError && (
        <p className="mt-4 border border-danger bg-danger/10 p-3 text-sm" role="alert">
          {cancelError}
        </p>
      )}
      <RunSummary run={run} />
      {confirming && (
        <CancelDialog
          canceling={canceling}
          onCancel={cancel}
          onClose={() => setConfirming(false)}
        />
      )}
    </section>
  );
}

function CancelDialog({
  canceling,
  onCancel,
  onClose,
}: {
  canceling: boolean;
  onCancel: () => Promise<void>;
  onClose: () => void;
}) {
  return (
    <Dialog
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      open
      title="Cancel evaluation run"
    >
      <p className="mt-3 text-sm">
        Pending work will be canceled. Completed scores remain available.
      </p>
      <div className="mt-6 flex justify-end gap-3">
        <Button autoFocus onClick={onClose}>
          Keep running
        </Button>
        <Button disabled={canceling} onClick={() => void onCancel()} variant="danger">
          Confirm cancellation
        </Button>
      </div>
    </Dialog>
  );
}

function RunSummary({ run }: { run: EvalRunResponse }) {
  return (
    <div className="mt-6 space-y-6">
      <dl className="grid gap-3 sm:grid-cols-4">
        <Count label="Total" value={run.total_items} />
        <Count label="Succeeded" value={run.succeeded_items} />
        <Count label="Failed" value={run.failed_items} />
        <Count label="Canceled" value={run.canceled_items} />
      </dl>
      <section>
        <h2 className="text-lg font-semibold">Score aggregates</h2>
        {Object.keys(run.aggregates).length === 0 ? (
          <p className="mt-3 text-sm text-muted">No aggregate scores yet.</p>
        ) : (
          <div className="mt-3 overflow-x-auto border border-line bg-surface">
            <table className="w-full text-left text-sm">
              <thead className="bg-canvas">
                <tr>
                  <th className="px-4 py-3">Scorer</th>
                  <th className="px-4 py-3">Mean</th>
                  <th className="px-4 py-3">Pass rate</th>
                  <th className="px-4 py-3">Samples</th>
                </tr>
              </thead>
              <tbody>
                {Object.entries(run.aggregates).map(([scorer, value]) => (
                  <tr className="border-t border-line" key={scorer}>
                    <td className="px-4 py-3">{scorer}</td>
                    <td className="px-4 py-3">{value.mean.toFixed(2)}</td>
                    <td className="px-4 py-3">{(value.pass_rate * 100).toFixed(0)}%</td>
                    <td className="px-4 py-3">{value.n}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}

function Count({ label, value }: { label: string; value: number }) {
  return (
    <div className="border border-line bg-surface p-4">
      <dt className="text-xs uppercase tracking-wide text-muted">{label}</dt>
      <dd className="mt-1 text-xl font-semibold">{value}</dd>
    </div>
  );
}
