import { useCallback, useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";

import { Problem } from "@/api/errors";
import { compareEvalRuns, getEvalRun } from "@/api/generated/sdk.gen";
import type {
  EvalRunItemResponse,
  EvalRunResponse,
  RunComparisonResponse,
  RunComparisonRow,
} from "@/api/generated/types.gen";
import { JsonView } from "@/components/json-view";
import { ScoreResult } from "@/components/score-result";
import { Button } from "@/components/ui/button";
import { StatusBadge } from "@/components/ui/status-badge";

export function RunComparison() {
  const { appId = "" } = useParams();
  const [searchParams, setSearchParams] = useSearchParams();
  const baselineID = searchParams.get("baseline") ?? "";
  const candidateID = searchParams.get("candidate") ?? "";
  const scorer = searchParams.get("scorer") ?? "";
  if (baselineID === "" || candidateID === "" || scorer === "" || baselineID === candidateID) {
    return <ComparisonError message="Choose two distinct terminal runs and a shared scorer." />;
  }
  const key = `${baselineID}:${candidateID}:${scorer}`;
  return (
    <ComparisonContent
      appID={appId}
      baselineID={baselineID}
      candidateID={candidateID}
      key={key}
      onSwap={() => setSearchParams({ baseline: candidateID, candidate: baselineID, scorer })}
      scorer={scorer}
    />
  );
}

function ComparisonContent({
  appID,
  baselineID,
  candidateID,
  onSwap,
  scorer,
}: {
  appID: string;
  baselineID: string;
  candidateID: string;
  onSwap: () => void;
  scorer: string;
}) {
  const [runs, setRuns] = useState<[EvalRunResponse, EvalRunResponse] | null>(null);
  const [comparison, setComparison] = useState<RunComparisonResponse | null>(null);
  const [pageCursor, setPageCursor] = useState<string>();
  const [history, setHistory] = useState<Array<string | undefined>>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    void Promise.all([
      getEvalRun({ path: { id: baselineID }, signal: controller.signal, throwOnError: true }),
      getEvalRun({ path: { id: candidateID }, signal: controller.signal, throwOnError: true }),
    ])
      .then(([baseline, candidate]) => {
        if (baseline.data.application_id !== appID || candidate.data.application_id !== appID) {
          setError("Both runs must belong to this application.");
          return;
        }
        setRuns([baseline.data, candidate.data]);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setError(problemMessage(reason, "Unable to load runs"));
      });
    return () => controller.abort();
  }, [appID, baselineID, candidateID]);

  const load = useCallback(
    async (cursor: string | undefined, signal: AbortSignal): Promise<void> => {
      try {
        const response = await compareEvalRuns({
          path: { id: baselineID },
          query: {
            other_run_id: candidateID,
            scorer,
            limit: 100,
            ...(cursor === undefined ? {} : { cursor }),
          },
          signal,
          throwOnError: true,
        });
        if (!signal.aborted) setComparison(response.data);
      } catch (reason) {
        if (!signal.aborted) setError(problemMessage(reason, "Unable to compare runs"));
      }
    },
    [baselineID, candidateID, scorer],
  );

  useEffect(() => {
    if (runs === null) return;
    const controller = new AbortController();
    setComparison(null);
    setError(null);
    void load(pageCursor, controller.signal);
    return () => controller.abort();
  }, [load, pageCursor, runs]);

  if (error !== null) return <ComparisonError message={error} />;
  if (runs === null || comparison === null)
    return <p className="text-sm text-muted">Loading run comparison...</p>;

  return (
    <section aria-labelledby="comparison-heading">
      <Link className="text-sm text-accent hover:underline" to={`/apps/${appID}/runs`}>
        Back to runs
      </Link>
      <div className="mt-4 flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.14em] text-muted">{scorer}</p>
          <h1 className="mt-1 text-2xl font-semibold" id="comparison-heading">
            {runs[0].name} vs {runs[1].name}
          </h1>
          <p className="mt-2 text-sm text-muted">Delta = candidate − baseline.</p>
        </div>
        <Button onClick={onSwap}>Swap baseline and candidate</Button>
      </div>
      <ComparisonSummary comparison={comparison} />
      <ComparisonRows items={comparison.items} names={[runs[0].name, runs[1].name]} />
      <PageControls
        history={history}
        nextCursor={comparison.next_cursor}
        onNext={(cursor) => {
          setHistory((current) => [...current, pageCursor]);
          setPageCursor(cursor);
        }}
        onPrevious={() => {
          setPageCursor(history.at(-1));
          setHistory((current) => current.slice(0, -1));
        }}
      />
    </section>
  );
}

function ComparisonSummary({ comparison }: { comparison: RunComparisonResponse }) {
  const summary = comparison.summary;
  return (
    <section aria-labelledby="comparison-summary-heading" className="mt-6">
      <h2 className="text-lg font-semibold" id="comparison-summary-heading">
        Paired summary
      </h2>
      <dl className="mt-3 grid gap-3 sm:grid-cols-3 lg:grid-cols-6">
        <Metric label="Matched" value={String(summary.n)} />
        <Metric label="Mean delta" value={formatDelta(summary.mean_delta)} />
        <Metric label="Changed cases" value={String(summary.changed_cases)} />
        <Metric label="Baseline only" value={String(summary.baseline_only)} />
        <Metric label="Candidate only" value={String(summary.candidate_only)} />
        <Metric label="Unscored" value={String(summary.unscored)} />
      </dl>
      {summary.n === 0 && (
        <p className="mt-3 text-sm text-warning">No unchanged cases have scores in both runs.</p>
      )}
      {comparison.warnings.length > 0 && (
        <ul aria-label="Comparison warnings" className="mt-4 space-y-2">
          {comparison.warnings.map((warning) => (
            <li className="border border-warning bg-warning/10 p-3 text-sm" key={warning}>
              {warning}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="border border-line bg-surface p-3">
      <dt className="text-xs uppercase tracking-wide text-muted">{label}</dt>
      <dd className="mt-1 font-mono text-lg">{value}</dd>
    </div>
  );
}

function ComparisonRows({ items, names }: { items: RunComparisonRow[]; names: [string, string] }) {
  if (items.length === 0) return <p className="mt-6 text-sm text-muted">No cases on this page.</p>;
  return (
    <div className="mt-6 space-y-3">
      {items.map((item) => {
        const label =
          item.baseline?.snapshot.external_id ??
          item.candidate?.snapshot.external_id ??
          item.dataset_item_id;
        return (
          <details className="border border-line bg-surface" key={item.dataset_item_id}>
            <summary className="cursor-pointer p-4 font-medium">
              {label} · {kindLabel(item.kind)} · {formatDelta(item.delta)}
            </summary>
            <div className="grid gap-4 border-t border-line p-4 lg:grid-cols-2">
              <ComparisonEvidence item={item.baseline} label={`Baseline: ${names[0]}`} />
              <ComparisonEvidence item={item.candidate} label={`Candidate: ${names[1]}`} />
            </div>
          </details>
        );
      })}
    </div>
  );
}

function ComparisonEvidence({ item, label }: { item: EvalRunItemResponse | null; label: string }) {
  if (item === null)
    return (
      <section>
        <h3 className="font-semibold">{label}</h3>
        <p className="mt-3 text-sm text-muted">Case is absent from this run.</p>
      </section>
    );
  return (
    <section>
      <div className="flex flex-wrap items-center gap-3">
        <h3 className="font-semibold">{label}</h3>
        <StatusBadge tone={item.status === "succeeded" ? "success" : "danger"}>
          {item.status}
        </StatusBadge>
      </div>
      {item.error && <p className="mt-3 text-sm text-danger">Execution error: {item.error}</p>}
      <div className="mt-3 space-y-3">
        <JsonView
          value={{
            context: item.snapshot.context,
            expected_output: item.snapshot.expected_output,
            generated_context: item.generated_context,
            generated_output: item.generated_output,
            input: item.snapshot.input,
            recorded_output: item.snapshot.output,
          }}
        />
        {item.scores.length === 0 ? (
          <p className="text-sm text-muted">No score recorded for the selected scorer.</p>
        ) : (
          item.scores.map((score) => <ScoreResult key={score.id} score={score} />)
        )}
      </div>
    </section>
  );
}

function PageControls({
  history,
  nextCursor,
  onNext,
  onPrevious,
}: {
  history: Array<string | undefined>;
  nextCursor: string | undefined;
  onNext: (cursor: string) => void;
  onPrevious: () => void;
}) {
  if (history.length === 0 && nextCursor === undefined) return null;
  return (
    <div className="mt-4 flex gap-2">
      {history.length > 0 && <Button onClick={onPrevious}>Previous cases</Button>}
      {nextCursor !== undefined && <Button onClick={() => onNext(nextCursor)}>Next cases</Button>}
    </div>
  );
}

function ComparisonError({ message }: { message: string }) {
  return (
    <p className="border border-danger bg-danger/10 p-4" role="alert">
      {message}
    </p>
  );
}

function problemMessage(reason: unknown, fallback: string): string {
  return reason instanceof Problem ? (reason.detail ?? reason.title) : fallback;
}

function formatDelta(value: number | null): string {
  if (value === null) return "Excluded";
  if (value > 0) return `+${value.toFixed(2)}`;
  return value.toFixed(2);
}

function kindLabel(kind: RunComparisonRow["kind"]): string {
  return kind.replaceAll("_", " ");
}
