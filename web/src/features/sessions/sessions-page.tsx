import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";

import { Problem } from "@/api/errors";
import { listSessions } from "@/api/generated/sdk.gen";
import type { SessionResponse } from "@/api/generated/types.gen";

export function useSessions(appId: string) {
  const [items, setItems] = useState<SessionResponse[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  const [loadedAppId, setLoadedAppId] = useState<string | null>(null);
  const generation = useRef(0);

  useEffect(() => {
    generation.current += 1;
    const controller = new AbortController();
    setLoading(true);
    setItems([]);
    setCursor(null);
    setError(null);
    void listSessions({
      query: { application_id: appId },
      signal: controller.signal,
      throwOnError: true,
    })
      .then(({ data }) => {
        if (controller.signal.aborted) return;
        setItems(data.items);
        setCursor(data.next_cursor ?? null);
        setLoadedAppId(appId);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) {
          setError(sessionError(reason));
          setLoadedAppId(appId);
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [appId, revision]);

  const loadMore = useCallback(async () => {
    if (cursor === null || loading) return;
    const request = generation.current;
    setLoading(true);
    setError(null);
    try {
      const { data } = await listSessions({
        query: { application_id: appId, cursor },
        throwOnError: true,
      });
      if (request !== generation.current) return;
      setItems((current) => [
        ...current,
        ...data.items.filter((item) => !current.some((previous) => previous.id === item.id)),
      ]);
      setCursor(data.next_cursor === cursor ? null : (data.next_cursor ?? null));
      if (data.next_cursor === cursor) setError("Session pagination stopped: repeated cursor.");
    } catch (reason) {
      if (request === generation.current) setError(sessionError(reason));
    } finally {
      if (request === generation.current) setLoading(false);
    }
  }, [appId, cursor, loading]);

  const visibleItems = loadedAppId === appId ? items : [];
  const visibleCursor = loadedAppId === appId ? cursor : null;
  const pending = loading || loadedAppId !== appId;
  const visibleError = loadedAppId === appId ? error : null;

  return {
    items: visibleItems,
    cursor: visibleCursor,
    loading: pending,
    error: visibleError,
    refresh: () => setRevision((value) => value + 1),
    loadMore,
  };
}

export function SessionsPage() {
  const { appId = "" } = useParams();
  const { items, cursor, loading, error, refresh, loadMore } = useSessions(appId);
  return (
    <section aria-labelledby="sessions-heading" className="sessions-page space-y-6">
      <SessionHeading onRefresh={refresh} />
      <SessionListStatus appId={appId} error={error} loading={loading} empty={items.length === 0} />
      {items.length > 0 && <SessionRows appId={appId} items={items} />}
      {cursor !== null && (
        <button
          className="session-control"
          disabled={loading}
          onClick={() => void loadMore()}
          type="button"
        >
          Load more sessions
        </button>
      )}
    </section>
  );
}

function SessionListStatus({
  appId,
  error,
  loading,
  empty,
}: {
  appId: string;
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
  if (loading) return <p role="status">Loading sessions…</p>;
  return <SessionEmpty appId={appId} />;
}

function SessionHeading({ onRefresh }: { onRefresh: () => void }) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p className="session-eyebrow">Conversation observability</p>
        <h1 className="session-page-title" id="sessions-heading">
          Sessions, not just spans.
        </h1>
        <p className="session-subtitle">
          Follow the conversation. Inspect the evidence behind every answer.
        </p>
      </div>
      <button className="session-control" onClick={onRefresh} type="button">
        Refresh
      </button>
    </div>
  );
}

function SessionEmpty({ appId }: { appId: string }) {
  return (
    <div className="rounded-xl border border-line bg-surface p-8 sm:p-12">
      <p className="font-mono text-xs uppercase tracking-widest text-accent">
        No captured sessions
      </p>
      <h2 className="mt-3 text-xl font-semibold">Your traces are still here.</h2>
      <p className="mt-2 max-w-xl text-sm leading-6 text-muted">
        Add <code>session.id</code> to the root span of each turn to connect traces into a session.
        Untagged traces remain in Traces.
      </p>
      <Link
        className="mt-5 inline-block text-sm font-medium text-accent hover:underline"
        to={`/apps/${appId}/traces`}
      >
        Explore all traces →
      </Link>
    </div>
  );
}

export function SessionRows({
  appId,
  items,
  selectedId,
}: {
  appId: string;
  items: SessionResponse[];
  selectedId?: string;
}) {
  return (
    <ul className="session-list" aria-label="Sessions">
      {items.map((session) => (
        <li
          key={session.id}
          className="session-list-item"
          data-selected={selectedId === session.id}
        >
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="min-w-0">
              <p className="font-mono text-xs uppercase tracking-widest text-accent">
                {session.turn_count} {session.turn_count === 1 ? "turn" : "turns"}
              </p>
              <Link
                aria-current={selectedId === session.id ? "page" : undefined}
                className="session-list-link"
                to={`/apps/${appId}/sessions/detail?session_id=${encodeURIComponent(session.id)}`}
              >
                {session.first_operation}
              </Link>
              <p className="session-list-id" title={session.id}>
                {session.id}
              </p>
            </div>
            <p className="session-list-time text-muted">
              Last activity{" "}
              <time dateTime={session.end_time}>{new Date(session.end_time).toLocaleString()}</time>
            </p>
          </div>
        </li>
      ))}
    </ul>
  );
}

export function sessionError(reason: unknown): string {
  return reason instanceof Problem
    ? (reason.detail ?? reason.title)
    : "Unable to load sessions. Try again.";
}
