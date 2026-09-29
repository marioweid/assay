import { useEffect, useRef, useState } from "react";

import { getTrace, listSessionTurns } from "@/api/generated/sdk.gen";
import type { SessionTurnResponse, TraceResponse } from "@/api/generated/types.gen";
import { sessionError } from "@/features/sessions/sessions-page";

export function useSessionTurns(appId: string, sessionId: string) {
  const [turns, setTurns] = useState<SessionTurnResponse[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [loadedScope, setLoadedScope] = useState<string | null>(null);
  const scope = JSON.stringify([appId, sessionId]);
  const generation = useRef(0);

  useEffect(() => {
    generation.current += 1;
    const controller = new AbortController();
    setTurns([]);
    setCursor(null);
    setError(null);
    setLoading(true);
    void listSessionTurns({
      query: { application_id: appId, session_id: sessionId },
      signal: controller.signal,
      throwOnError: true,
    })
      .then(({ data }) => {
        if (controller.signal.aborted) return;
        setTurns(data.items);
        setCursor(data.next_cursor ?? null);
        setLoadedScope(scope);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) {
          setError(sessionError(reason));
          setLoadedScope(scope);
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [appId, sessionId]);

  async function loadMore() {
    if (cursor === null || loading) return;
    const request = generation.current;
    setLoading(true);
    setError(null);
    try {
      const { data } = await listSessionTurns({
        query: { application_id: appId, session_id: sessionId, cursor },
        throwOnError: true,
      });
      if (request !== generation.current) return;
      setTurns((current) => [
        ...current,
        ...data.items.filter((turn) => !current.some((previous) => previous.id === turn.id)),
      ]);
      setCursor(data.next_cursor === cursor ? null : (data.next_cursor ?? null));
      if (data.next_cursor === cursor) setError("Turn pagination stopped: repeated cursor.");
    } catch (reason) {
      if (request === generation.current) setError(sessionError(reason));
    } finally {
      if (request === generation.current) setLoading(false);
    }
  }

  return {
    turns: loadedScope === scope ? turns : [],
    cursor: loadedScope === scope ? cursor : null,
    loading: loading || loadedScope !== scope,
    error: loadedScope === scope ? error : null,
    loadMore,
  };
}

export function useSelectedTrace(appId: string, selectedId: string | null) {
  const [trace, setTrace] = useState<TraceResponse | null>(null);
  const [loadingTrace, setLoadingTrace] = useState(false);
  const [traceError, setTraceError] = useState<string | null>(null);
  const [loadedScope, setLoadedScope] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  const scope = JSON.stringify([appId, selectedId]);

  useEffect(() => {
    setTrace(null);
    setTraceError(null);
    setLoadedScope(null);
    if (selectedId === null) {
      setLoadingTrace(false);
      return;
    }
    const controller = new AbortController();
    setLoadingTrace(true);
    void getTrace({ path: { id: selectedId }, signal: controller.signal, throwOnError: true })
      .then(({ data }) => {
        if (controller.signal.aborted) return;
        if (data.application_id !== appId) {
          setTraceError("This trace does not belong to this application.");
          return;
        }
        setTrace(data);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setTraceError(sessionError(reason));
      })
      .finally(() => {
        if (controller.signal.aborted) return;
        setLoadedScope(scope);
        setLoadingTrace(false);
      });
    return () => controller.abort();
  }, [appId, selectedId, revision, scope]);

  return {
    trace: loadedScope === scope ? trace : null,
    loadingTrace,
    traceError: loadedScope === scope ? traceError : null,
    refreshTrace: () => setRevision((value) => value + 1),
  };
}
