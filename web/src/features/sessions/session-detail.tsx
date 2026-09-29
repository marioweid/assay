import { useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";

import { useSelectedTrace, useSessionTurns } from "@/features/sessions/session-data";
import { RecentSessions } from "@/features/sessions/session-recent";
import { SessionSummary, SessionThread } from "@/features/sessions/session-thread";
import { SessionTimeline } from "@/features/sessions/session-timeline";
import { SessionTracePanel } from "@/features/sessions/session-trace-panel";
import type { SpanKey } from "@/features/traces/conversation-model";
import { SpanWaterfall } from "@/features/traces/span-waterfall";

export function SessionDetail() {
  const { appId = "" } = useParams();
  const [searchParams] = useSearchParams();
  const sessionId = searchParams.get("session_id") ?? "";
  return (
    <SessionWorkbench
      key={JSON.stringify([appId, sessionId])}
      appId={appId}
      sessionId={sessionId}
    />
  );
}

function SessionWorkbench({ appId, sessionId }: { appId: string; sessionId: string }) {
  const { turns, cursor, loading, error, loadMore } = useSessionTurns(appId, sessionId);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const { trace, loadingTrace, traceError, refreshTrace } = useSelectedTrace(appId, selectedId);
  const [selectedSpan, setSelectedSpan] = useState<SpanKey | null>(null);
  const [mode, setMode] = useState<"conversation" | "timeline">("conversation");

  function selectTurn(id: string): void {
    setSelectedId(id);
    setSelectedSpan(null);
    document.getElementById("session-inspector")?.focus();
  }

  return (
    <section aria-labelledby="session-heading" className="sessions-page">
      <SessionHeading appId={appId} />
      <SessionSummary turns={turns} hasMore={cursor !== null} sessionId={sessionId} />
      <SessionTurnStatus error={error} loading={loading} empty={turns.length === 0} />
      {turns.length > 0 && (
        <div className="session-workspace">
          <RecentSessions appId={appId} sessionId={sessionId} />
          <section aria-label="Session conversation" className="session-panel session-thread">
            <div className="session-thread-heading">
              <ThreadHeading
                name={turns[0]?.root_name}
                count={turns.length}
                hasMore={cursor !== null}
              />
              <SessionViewSwitch mode={mode} onChange={setMode} />
            </div>
            {mode === "conversation" ? (
              <SessionThread
                appId={appId}
                turns={turns}
                selectedId={selectedId}
                onSelect={selectTurn}
              />
            ) : (
              <SessionTimeline onSelect={selectTurn} selectedId={selectedId} turns={turns} />
            )}
            {cursor !== null && (
              <div className="session-panel-note">
                <button
                  className="session-control"
                  disabled={loading}
                  onClick={() => void loadMore()}
                  type="button"
                >
                  Load more turns
                </button>
              </div>
            )}
            <div className="session-inline-timing">
              <h3>Selected turn · Timing</h3>
              {trace === null ? (
                <p>Select a message to see its execution timing.</p>
              ) : (
                <SpanWaterfall
                  onSelect={setSelectedSpan}
                  selectedKey={selectedSpan}
                  spans={trace.spans ?? []}
                />
              )}
            </div>
          </section>
          <SessionTracePanel
            appId={appId}
            loading={loadingTrace}
            onSelectSpan={setSelectedSpan}
            onRefresh={refreshTrace}
            selectedId={selectedId}
            selectedSpan={selectedSpan}
            trace={trace}
            traceError={traceError}
          />
        </div>
      )}
      <p className="session-footnote">
        Select either message in a turn to inspect its source trace, context and timing.
      </p>
    </section>
  );
}

function SessionTurnStatus({
  error,
  loading,
  empty,
}: {
  error: string | null;
  loading: boolean;
  empty: boolean;
}) {
  if (error !== null)
    return (
      <p className="session-notice" role="alert">
        {error}
      </p>
    );
  if (!empty) return null;
  if (loading) return <p role="status">Loading turns…</p>;
  return (
    <p className="session-notice">No turns in this session. The traces may have been removed.</p>
  );
}

function ThreadHeading({
  name,
  count,
  hasMore,
}: {
  name: string | undefined;
  count: number;
  hasMore: boolean;
}) {
  return (
    <div>
      <h2>{name ?? "Conversation"}</h2>
      <p>
        {count} loaded {count === 1 ? "turn" : "turns"}
        {hasMore ? " · more available" : ""}
      </p>
    </div>
  );
}

function SessionHeading({ appId }: { appId: string }) {
  return (
    <div className="session-page-heading">
      <div>
        <p className="session-eyebrow">Conversation observability</p>
        <h1 className="session-page-title" id="session-heading">
          Sessions, not just spans.
        </h1>
        <p className="session-subtitle">
          Follow the conversation. Inspect the evidence behind every answer.
        </p>
      </div>
      <Link className="session-control" to={`/apps/${appId}/sessions`}>
        ← All sessions
      </Link>
    </div>
  );
}

function SessionViewSwitch({
  mode,
  onChange,
}: {
  mode: "conversation" | "timeline";
  onChange: (mode: "conversation" | "timeline") => void;
}) {
  return (
    <div className="session-view-switch" aria-label="Session view">
      {(["conversation", "timeline"] as const).map((view) => (
        <button
          aria-pressed={mode === view}
          key={view}
          onClick={() => onChange(view)}
          type="button"
        >
          {view === "timeline" ? "Timeline" : "Conversation"}
        </button>
      ))}
    </div>
  );
}
