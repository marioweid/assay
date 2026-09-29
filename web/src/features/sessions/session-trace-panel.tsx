import { RefreshCw } from "lucide-react";
import { Link } from "react-router";

import type {
  ScoreResponse,
  ScoringTaskResponse,
  SpanResponse,
  TraceResponse,
} from "@/api/generated/types.gen";
import { JsonView } from "@/components/json-view";
import { ScoreResult } from "@/components/score-result";
import { StatusBadge } from "@/components/ui/status-badge";
import { conversationCalls, spanKey, type SpanKey } from "@/features/traces/conversation-model";
import { ConversationView } from "@/features/traces/conversation-view";
import { RetrievalContext } from "@/features/traces/retrieval-context";
import { turnDuration } from "@/features/sessions/session-thread";

type SessionTracePanelProps = {
  appId: string;
  loading: boolean;
  onSelectSpan: (key: SpanKey | null) => void;
  onRefresh: () => void;
  selectedId: string | null;
  selectedSpan: SpanKey | null;
  trace: TraceResponse | null;
  traceError: string | null;
};

export function SessionTracePanel({
  appId,
  loading,
  onSelectSpan,
  onRefresh,
  selectedId,
  selectedSpan,
  trace,
  traceError,
}: SessionTracePanelProps) {
  return (
    <aside
      aria-label="Selected trace inspector"
      className="session-panel session-inspector"
      id="session-inspector"
      tabIndex={-1}
    >
      <div className="session-panel-heading">
        <div>
          <h2>Trace inspection</h2>
          <p>Evidence behind the selected turn</p>
        </div>
        <button
          aria-label="Refresh source trace"
          title="Refresh source trace"
          className="session-icon-button"
          disabled={loading || selectedId === null}
          onClick={onRefresh}
          type="button"
        >
          <RefreshCw aria-hidden="true" size={15} />
        </button>
      </div>
      <div className="session-inspector-body">
        {selectedId === null ? (
          <p className="text-sm text-muted">
            Select a message or timeline row to inspect its source trace.
          </p>
        ) : (
          <>
            <p className="session-eyebrow">Source trace</p>
            <p className="session-trace-id" title={selectedId}>
              {selectedId}
            </p>
            {loading && (
              <p className="mt-3 text-sm text-muted" role="status">
                Loading trace…
              </p>
            )}
            {traceError !== null && (
              <p className="mt-3 text-sm text-danger" role="alert">
                {traceError}
              </p>
            )}
            {trace !== null && (
              <TraceInspector
                appId={appId}
                onSelectSpan={onSelectSpan}
                selectedSpan={selectedSpan}
                trace={trace}
              />
            )}
          </>
        )}
      </div>
    </aside>
  );
}

function TraceInspector({
  appId,
  trace,
  selectedSpan,
  onSelectSpan,
}: {
  appId: string;
  trace: TraceResponse;
  selectedSpan: SpanKey | null;
  onSelectSpan: (key: SpanKey | null) => void;
}) {
  const spans = trace.spans ?? [];
  const selected = selectedSpan === null ? null : findSpan(spans, selectedSpan);
  const scores = (trace.scores ?? []).filter(
    (score) => selected === null || score.span_id === undefined || score.span_id === selected.id,
  );
  return (
    <div className="mt-3 min-w-0 space-y-5 text-sm">
      <div>
        <h3 className="break-words font-semibold">{trace.root_name}</h3>
        <p className="mt-1 text-xs text-muted">
          {trace.span_count} spans · {trace.status}
        </p>
      </div>
      <dl className="session-stat-row">
        <div>
          <dt>Duration</dt>
          <dd>{turnDuration(trace.start_time, trace.end_time)}</dd>
        </div>
        <div>
          <dt>Tokens used</dt>
          <dd>{trace.total_tokens.toLocaleString()}</dd>
        </div>
      </dl>
      <ExecutionSpans spans={spans} selectedSpan={selectedSpan} onSelect={onSelectSpan} />
      <RetrievalContext spans={spans} />
      <SourceScores scores={scores} selected={selected} tasks={trace.scoring_tasks ?? []} />
      {selected !== null && <SelectedSpan onClear={() => onSelectSpan(null)} span={selected} />}
      <SourceEvidence onSelectSpan={onSelectSpan} spans={spans} trace={trace} selected={selected} />
      <Link className="session-control w-full" to={`/apps/${appId}/traces/${trace.id}`}>
        Open full trace and scores →
      </Link>
    </div>
  );
}

function SourceScores({
  scores,
  selected,
  tasks,
}: {
  scores: readonly ScoreResponse[];
  selected: SpanResponse | null;
  tasks: readonly ScoringTaskResponse[];
}) {
  const activeTasks = tasks.filter((task) => task.status !== "succeeded");
  return (
    <section aria-label="Source scores" className="space-y-2 border-t border-line pt-4">
      <h3 className="break-words font-semibold">
        {selected === null ? "Trace scores" : `Scores for ${selected.name}`} · {scores.length}
      </h3>
      {activeTasks.map((task) => (
        <p className="text-xs text-muted" key={task.id}>
          Scoring {task.scorer}: {task.status}
          {task.error ? ` · ${task.error}` : ""}
        </p>
      ))}
      {scores.length === 0 && activeTasks.length === 0 && (
        <p className="text-sm text-muted">
          {selected === null ? "No scores recorded for this trace." : "No scores for this span."}
        </p>
      )}
      {scores.map((score) => (
        <details className="rounded-lg border border-line bg-canvas" key={score.id}>
          <summary className="flex cursor-pointer flex-wrap items-center gap-2 p-3">
            <span className="mr-auto font-medium">{score.scorer}</span>
            <span className="font-mono text-xs text-muted">
              {score.span_id === undefined ? "Trace" : "Span"}
            </span>
            <StatusBadge tone={score.passed ? "success" : "danger"}>
              {score.passed ? "Passed" : "Failed"}
            </StatusBadge>
            <strong className="font-mono">{score.value.toFixed(2)}</strong>
          </summary>
          <div className="p-2 pt-0">
            <ScoreResult score={score} />
          </div>
        </details>
      ))}
    </section>
  );
}

function SelectedSpan({ onClear, span }: { onClear: () => void; span: SpanResponse }) {
  return (
    <section
      aria-label="Selected span"
      className="rounded-lg border border-accent/40 bg-canvas p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="break-words font-medium">{span.name}</h3>
        <button className="text-xs text-accent hover:underline" onClick={onClear} type="button">
          Show all spans and scores
        </button>
      </div>
      <dl className="mt-2 grid grid-cols-2 gap-2 text-xs">
        <div>
          <dt className="text-muted">Status</dt>
          <dd>{span.status_code}</dd>
        </div>
        <div>
          <dt className="text-muted">Duration</dt>
          <dd>{span.duration_ms.toFixed(1)} ms</dd>
        </div>
        <div>
          <dt className="text-muted">Input tokens</dt>
          <dd>{span.input_tokens}</dd>
        </div>
        <div>
          <dt className="text-muted">Output tokens</dt>
          <dd>{span.output_tokens}</dd>
        </div>
      </dl>
    </section>
  );
}

function SourceEvidence({
  onSelectSpan,
  spans,
  trace,
  selected,
}: {
  onSelectSpan: (key: SpanKey | null) => void;
  spans: readonly SpanResponse[];
  trace: TraceResponse;
  selected: SpanResponse | null;
}) {
  const calls = conversationCalls(spans);
  return (
    <details className="border-t border-line pt-4">
      <summary className="flex cursor-pointer flex-wrap gap-2 font-medium">
        <span>Model calls and captured context</span>
        <span className="text-muted">({calls.length})</span>
      </summary>
      <div className="mt-4 space-y-4">
        {calls.length > 0 ? (
          <ConversationView
            calls={calls}
            onSelectSpan={(key) => {
              if (key !== null) onSelectSpan(key);
            }}
            selectedSpanKey={null}
          />
        ) : (
          <p className="text-sm text-muted">
            No model messages captured. Span metadata remains available.
          </p>
        )}
        <details className="border-t border-line pt-3">
          <summary className="cursor-pointer text-sm">
            Raw {selected === null ? "trace" : "span"} attributes
          </summary>
          <div className="mt-2">
            <JsonView value={selected?.attributes ?? trace.attributes} />
          </div>
        </details>
      </div>
    </details>
  );
}

function ExecutionSpans({
  spans,
  selectedSpan,
  onSelect,
}: {
  spans: readonly SpanResponse[];
  selectedSpan: SpanKey | null;
  onSelect: (key: SpanKey) => void;
}) {
  const rows: SpanResponse[] = [];
  const pending = [...spans].reverse();
  while (pending.length > 0) {
    const span = pending.pop();
    if (span === undefined) continue;
    rows.push(span);
    pending.push(...[...(span.children ?? [])].reverse());
  }
  return (
    <section aria-label="Execution" className="session-execution">
      <h3 className="session-eyebrow">Execution</h3>
      {rows.map((span) => (
        <button
          key={spanKey(span)}
          aria-pressed={selectedSpan === spanKey(span)}
          onClick={() => onSelect(spanKey(span))}
          type="button"
        >
          <span className="truncate">↳ {span.name}</span>
          <span>{turnDuration(span.start_time, span.end_time)}</span>
        </button>
      ))}
    </section>
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
