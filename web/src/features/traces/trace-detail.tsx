import { useCallback, useEffect, useState } from "react";
import type { KeyboardEvent } from "react";
import { Link, useParams, useSearchParams } from "react-router";

import { Problem } from "@/api/errors";
import { getTrace } from "@/api/generated/sdk.gen";
import type { SpanResponse, TraceResponse } from "@/api/generated/types.gen";
import { JsonView } from "@/components/json-view";
import { ScoreResult } from "@/components/score-result";
import { SpanTree } from "@/features/traces/span-tree";
import { conversationCalls, spanKey, type SpanKey } from "@/features/traces/conversation-model";
import { ConversationView } from "@/features/traces/conversation-view";
import { RetrievalContext } from "@/features/traces/retrieval-context";
import { SpanWaterfall } from "@/features/traces/span-waterfall";
import { TraceActions } from "@/features/traces/trace-actions";

const tabs = ["Overview", "Attributes", "Events", "Scores"] as const;
type Tab = (typeof tabs)[number];

export function TraceDetail() {
  const { appId = "", traceId = "" } = useParams();
  const [searchParams] = useSearchParams();
  const [trace, setTrace] = useState<TraceResponse | null>(null);
  const [selected, setSelected] = useState<SpanResponse | null>(null);
  const [tab, setTab] = useState<Tab>(searchParams.get("tab") === "scores" ? "Scores" : "Overview");
  const [error, setError] = useState<string | null>(null);
  const scorer = searchParams.get("scorer");

  const loadTrace = useCallback(async (): Promise<void> => {
    const response = await getTrace({ path: { id: traceId }, throwOnError: true });
    if (response.data.application_id !== appId) {
      setError("Trace does not belong to this application");
      return;
    }
    setError(null);
    setTrace(response.data);
  }, [appId, traceId]);

  const refreshTrace = async (): Promise<void> => {
    try {
      await loadTrace();
    } catch (reason) {
      setError(reason instanceof Problem ? reason.title : "Unable to load trace");
    }
  };

  useEffect(() => {
    const controller = new AbortController();
    setTrace(null);
    setSelected(null);
    setError(null);
    void getTrace({ path: { id: traceId }, signal: controller.signal, throwOnError: true })
      .then((response) => {
        if (controller.signal.aborted) return;
        if (response.data.application_id !== appId) {
          setError("Trace does not belong to this application");
          return;
        }
        setTrace(response.data);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted)
          setError(reason instanceof Problem ? reason.title : "Unable to load trace");
      });
    return () => controller.abort();
  }, [appId, traceId]);

  if (error !== null)
    return (
      <p className="border border-danger bg-danger/10 p-4" role="alert">
        {error}
      </p>
    );
  if (trace === null) return <p className="text-muted">Loading trace...</p>;
  const scores = (trace.scores ?? []).filter(
    (score) =>
      (selected === null || score.span_id === selected.id) &&
      (scorer === null || score.scorer === scorer),
  );
  return (
    <section aria-labelledby="trace-heading">
      <Link className="text-sm text-accent hover:underline" to={`/apps/${appId}/traces`}>
        Back to traces
      </Link>
      <div className="mt-4 flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="font-mono text-xs text-muted">{trace.id}</p>
          <h1 className="mt-1 text-2xl font-semibold" id="trace-heading">
            {trace.root_name}
          </h1>
        </div>
        <button
          className="border border-line px-3 py-2 text-sm"
          onClick={() => void refreshTrace()}
          type="button"
        >
          Refresh trace
        </button>
      </div>
      <TraceActions appId={appId} onChanged={loadTrace} trace={trace} />
      <ScoringTasks tasks={trace.scoring_tasks ?? []} />
      <div className="mt-6 grid gap-5 lg:grid-cols-[minmax(16rem,0.75fr)_minmax(0,1.5fr)]">
        <aside className="border border-line bg-surface p-3">
          <div className="mb-3 flex items-center justify-between">
            <h2 className="font-semibold">Span tree</h2>
            <button className="text-xs text-accent" onClick={() => setSelected(null)}>
              Trace summary
            </button>
          </div>
          <SpanTree
            onSelect={setSelected}
            selectedID={selected?.id ?? null}
            spans={trace.spans ?? []}
          />
        </aside>
        <div className="min-w-0 border border-line bg-surface">
          <div className="border-b border-line p-4">
            <p className="text-xs uppercase tracking-wide text-muted">
              {selected === null ? "Trace" : "Span"}
            </p>
            <h2 className="mt-1 font-semibold">{selected?.name ?? trace.root_name}</h2>
          </div>
          <TraceTabs onSelect={setTab} selected={tab} />
          <div
            aria-labelledby={`trace-tab-${tab.toLowerCase()}`}
            className="p-4"
            id="trace-tabpanel"
            role="tabpanel"
          >
            <DetailPanel
              onSelect={setSelected}
              scores={scores}
              selected={selected}
              tab={tab}
              trace={trace}
            />
          </div>
        </div>
      </div>
    </section>
  );
}

function ScoringTasks({ tasks }: { tasks: NonNullable<TraceResponse["scoring_tasks"]> }) {
  if (tasks.length === 0) return null;
  return (
    <section
      className="mt-4 border border-line bg-surface p-4"
      aria-labelledby="scoring-tasks-heading"
    >
      <h2 className="font-semibold" id="scoring-tasks-heading">
        Scoring tasks
      </h2>
      <ul className="mt-2 space-y-1 text-sm">
        {tasks.map((task) => (
          <li key={task.id}>
            {task.scorer} — {task.status}
            {task.error === undefined ? "" : `: ${task.error}`}
          </li>
        ))}
      </ul>
    </section>
  );
}

function TraceTabs({ onSelect, selected }: { onSelect: (tab: Tab) => void; selected: Tab }) {
  function handleKeyDown(event: KeyboardEvent<HTMLButtonElement>): void {
    const currentIndex = tabs.indexOf(selected);
    let nextIndex: number | null = null;
    if (event.key === "ArrowRight") nextIndex = (currentIndex + 1) % tabs.length;
    if (event.key === "ArrowLeft") nextIndex = (currentIndex - 1 + tabs.length) % tabs.length;
    if (event.key === "Home") nextIndex = 0;
    if (event.key === "End") nextIndex = tabs.length - 1;
    if (nextIndex === null) return;
    event.preventDefault();
    const next = tabs[nextIndex];
    if (next === undefined) return;
    onSelect(next);
    event.currentTarget.parentElement
      ?.querySelectorAll<HTMLButtonElement>('[role="tab"]')
      .item(nextIndex)
      .focus();
  }

  return (
    <div
      aria-label="Trace details"
      className="flex overflow-x-auto border-b border-line"
      role="tablist"
    >
      {tabs.map((name) => (
        <button
          aria-controls="trace-tabpanel"
          aria-selected={selected === name}
          className={`px-4 py-3 text-sm ${selected === name ? "border-b-2 border-accent text-accent" : "text-muted"}`}
          id={`trace-tab-${name.toLowerCase()}`}
          key={name}
          onClick={() => onSelect(name)}
          onKeyDown={handleKeyDown}
          role="tab"
          tabIndex={selected === name ? 0 : -1}
        >
          {name}
        </button>
      ))}
    </div>
  );
}

type DetailPanelProps = {
  onSelect: (span: SpanResponse | null) => void;
  scores: NonNullable<TraceResponse["scores"]>;
  selected: SpanResponse | null;
  tab: Tab;
  trace: TraceResponse;
};

function DetailPanel({ onSelect, scores, selected, tab, trace }: DetailPanelProps) {
  if (tab === "Attributes") return <JsonView value={selected?.attributes ?? trace.attributes} />;
  if (tab === "Events") return <JsonView value={selected?.events ?? []} />;
  if (tab === "Scores")
    return scores.length === 0 ? (
      <p className="text-sm text-muted">No scores for this selection.</p>
    ) : (
      <div className="space-y-3">
        {scores.map((score) => (
          <ScoreResult key={score.id} score={score} />
        ))}
      </div>
    );
  return (
    <>
      <dl className="grid gap-4 text-sm sm:grid-cols-2">
        <Info label="Status" value={selected?.status_code ?? trace.status} />
        <Info label="Started" value={selected?.start_time ?? trace.start_time} />
        <Info
          label="Duration"
          value={
            selected === null
              ? `${new Date(trace.end_time).getTime() - new Date(trace.start_time).getTime()} ms`
              : `${selected.duration_ms} ms`
          }
        />
        <Info
          label="Tokens"
          value={
            selected === null
              ? String(trace.total_tokens)
              : String(selected.input_tokens + selected.output_tokens)
          }
        />
      </dl>
      <div className="mt-6 grid gap-5 xl:grid-cols-2">
        <ConversationView
          calls={conversationCalls(selected === null ? (trace.spans ?? []) : [selected])}
          onSelectSpan={(key) => onSelect(key === null ? null : findSpan(trace.spans ?? [], key))}
          selectedSpanKey={selected === null ? null : spanKey(selected)}
        />
        <SpanWaterfall
          onSelect={(key) => onSelect(findSpan(trace.spans ?? [], key))}
          spans={trace.spans ?? []}
        />
      </div>
      <RetrievalContext spans={selected === null ? (trace.spans ?? []) : [selected]} />
    </>
  );
}

function findSpan(spans: readonly SpanResponse[], key: SpanKey): SpanResponse | null {
  const pending = [...spans];
  while (pending.length > 0) {
    const span = pending.pop();
    if (span === undefined) continue;
    if (spanKey(span) === key) return span;
    pending.push(...(span.children ?? []));
  }
  return null;
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs uppercase tracking-wide text-muted">{label}</dt>
      <dd className="mt-1 font-medium">{value}</dd>
    </div>
  );
}
