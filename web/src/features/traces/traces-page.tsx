import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router";

import { Problem } from "@/api/errors";
import { listTraces } from "@/api/generated/sdk.gen";
import type { ListTracesData, TraceListResponse } from "@/api/generated/types.gen";
import {
  formatLocalDateTime,
  parseLocalDateTime,
  readTraceFilters,
  relativeTraceFilters,
  serializeTraceFilters,
  type TraceFilters,
  type TraceRange,
  type TraceScorer,
} from "@/features/traces/trace-filters";

type ListTracesQuery = NonNullable<ListTracesData["query"]>;

type TraceResults = {
  error: string | null;
  items: TraceListResponse[];
  loadMore: () => Promise<void>;
  loading: boolean;
  nextCursor: string | null;
};

function useTraceResults(
  appId: string,
  filters: TraceFilters,
  filterSearch: string,
  refreshNumber: number,
): TraceResults {
  const requestNumber = useRef(0);
  const activeRequest = useRef<AbortController | null>(null);
  const [items, setItems] = useState<TraceListResponse[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [seenCursors, setSeenCursors] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    const currentRequest = ++requestNumber.current;
    activeRequest.current?.abort();
    activeRequest.current = controller;
    setLoading(true);
    setError(null);
    setItems([]);
    setNextCursor(null);
    setSeenCursors(new Set());
    void listTraces({
      query: traceQuery(appId, filters),
      signal: controller.signal,
      throwOnError: true,
    })
      .then((response) => {
        if (requestNumber.current !== currentRequest || controller.signal.aborted) return;
        setItems(uniqueTraces(response.data.items ?? []));
        setNextCursor(response.data.next_cursor ?? null);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted && requestNumber.current === currentRequest) {
          setError(errorMessage(reason, "Unable to load traces"));
        }
      })
      .finally(() => {
        if (requestNumber.current === currentRequest) setLoading(false);
      });
    return () => activeRequest.current?.abort();
  }, [appId, filterSearch, refreshNumber]);

  const loadMore = useCallback(async (): Promise<void> => {
    if (nextCursor === null) return;
    const cursor = nextCursor;
    const consumed = new Set(seenCursors).add(cursor);
    const controller = new AbortController();
    const currentRequest = ++requestNumber.current;
    activeRequest.current = controller;
    setLoading(true);
    setError(null);
    try {
      const response = await listTraces({
        query: { ...traceQuery(appId, filters), cursor },
        signal: controller.signal,
        throwOnError: true,
      });
      if (requestNumber.current !== currentRequest || controller.signal.aborted) return;
      const receivedCursor = response.data.next_cursor ?? null;
      setItems((current) => uniqueTraces([...current, ...(response.data.items ?? [])]));
      setSeenCursors(consumed);
      if (receivedCursor !== null && consumed.has(receivedCursor)) {
        setError("The server returned a repeated cursor. Pagination stopped.");
        setNextCursor(null);
      } else setNextCursor(receivedCursor);
    } catch (reason) {
      if (!controller.signal.aborted && requestNumber.current === currentRequest) {
        setError(errorMessage(reason, "Unable to load more traces"));
      }
    } finally {
      if (requestNumber.current === currentRequest) setLoading(false);
    }
  }, [appId, filters, nextCursor, seenCursors]);

  return { error, items, loadMore, loading, nextCursor };
}

export function TracesPage() {
  const { appId = "" } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const now = useRef(new Date());
  const [search, setSearch] = useState("");
  const [refreshNumber, setRefreshNumber] = useState(0);
  const filters = useMemo(
    () => readTraceFilters(new URLSearchParams(location.search), now.current),
    [location.search],
  );
  const filtersRef = useRef(filters);
  filtersRef.current = filters;
  const filterSearch = serializeTraceFilters(filters);
  const updateFilters = useCallback(
    (update: Partial<TraceFilters>): void => {
      navigate({ search: serializeTraceFilters({ ...filtersRef.current, ...update }) });
    },
    [navigate],
  );

  useEffect(() => {
    if (location.search !== filterSearch) navigate({ search: filterSearch }, { replace: true });
  }, [filterSearch, location.search, navigate]);

  useEffect(() => setSearch(filters.q), [filters.q]);

  useEffect(() => {
    if (search === filters.q) return undefined;
    const timeout = window.setTimeout(() => updateFilters({ q: search }), 250);
    return () => window.clearTimeout(timeout);
  }, [filters.q, search, updateFilters]);

  const { error, items, loadMore, loading, nextCursor } = useTraceResults(
    appId,
    filters,
    filterSearch,
    refreshNumber,
  );

  function updateRange(range: TraceRange): void {
    if (range === "custom") {
      updateFilters({ range });
      return;
    }
    updateFilters({ range, ...relativeTraceFilters(range, new Date()) });
  }

  function refresh(): void {
    if (filters.range === "custom") {
      setRefreshNumber((current) => current + 1);
      return;
    }
    updateFilters({ ...relativeTraceFilters(filters.range, new Date()) });
  }

  return (
    <TraceTable
      appId={appId}
      error={error}
      filters={filters}
      items={items}
      loading={loading}
      nextCursor={nextCursor}
      onFilters={updateFilters}
      onLoadMore={loadMore}
      onRange={updateRange}
      onRefresh={refresh}
      onSearch={setSearch}
      search={search}
    />
  );
}

type TraceTableProps = {
  appId: string;
  error: string | null;
  filters: TraceFilters;
  items: TraceListResponse[];
  loading: boolean;
  nextCursor: string | null;
  onFilters: (update: Partial<TraceFilters>) => void;
  onLoadMore: () => Promise<void>;
  onRange: (range: TraceRange) => void;
  onRefresh: () => void;
  onSearch: (value: string) => void;
  search: string;
};

function TraceTable(props: TraceTableProps) {
  return (
    <section aria-labelledby="traces-heading">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.14em] text-muted">Observability</p>
          <h1 className="mt-1 text-2xl font-semibold" id="traces-heading">
            Traces
          </h1>
        </div>
        <button
          className="border border-line bg-surface px-3 py-2 text-sm font-medium"
          onClick={props.onRefresh}
          type="button"
        >
          Refresh
        </button>
      </div>
      <TraceFilterControls {...props} />
      {props.error && (
        <p className="mt-4 border border-amber-300 bg-amber-50 p-3 text-sm" role="alert">
          {props.error}
        </p>
      )}
      {props.loading && props.items.length === 0 && (
        <p className="mt-8 text-muted">Loading traces...</p>
      )}
      {!props.loading && props.error === null && props.items.length === 0 && (
        <p className="mt-8 text-muted">No traces found.</p>
      )}
      {props.items.length > 0 && <TraceRows appId={props.appId} items={props.items} />}
      {props.nextCursor !== null && (
        <button
          className="mt-4 border border-line bg-surface px-4 py-2 text-sm font-medium disabled:opacity-50"
          disabled={props.loading}
          onClick={() => void props.onLoadMore()}
          type="button"
        >
          Load more
        </button>
      )}
    </section>
  );
}

function TraceFilterControls(props: TraceTableProps) {
  const setScorer = (value: string): void => {
    props.onFilters(
      value === "" ? { passed: undefined, scorer: undefined } : { scorer: value as TraceScorer },
    );
  };
  return (
    <div className="mt-4 flex flex-wrap gap-3 text-sm text-muted">
      <label>
        Search
        <input
          className="ml-2 border border-line bg-surface px-3 py-2 text-ink"
          onChange={(event) => props.onSearch(event.target.value)}
          value={props.search}
        />
      </label>
      <label>
        Status
        <select
          className="ml-2 border border-line bg-surface px-2 py-2 text-ink"
          onChange={(event) => props.onFilters({ status: event.target.value })}
          value={props.filters.status}
        >
          <option value="">All</option>
          <option value="ok">OK</option>
          <option value="error">Error</option>
        </select>
      </label>
      <label>
        Scorer
        <select
          className="ml-2 border border-line bg-surface px-2 py-2 text-ink"
          onChange={(event) => setScorer(event.target.value)}
          value={props.filters.scorer ?? ""}
        >
          <option value="">All</option>
          <option value="groundedness">Groundedness</option>
          <option value="correctness">Correctness</option>
        </select>
      </label>
      <label>
        Pass
        <select
          className="ml-2 border border-line bg-surface px-2 py-2 text-ink"
          disabled={props.filters.scorer === undefined}
          onChange={(event) =>
            props.onFilters({
              passed: event.target.value === "" ? undefined : event.target.value === "true",
            })
          }
          value={props.filters.passed === undefined ? "" : String(props.filters.passed)}
        >
          <option value="">All</option>
          <option value="true">Passed</option>
          <option value="false">Failed</option>
        </select>
      </label>
      <label>
        Range
        <select
          className="ml-2 border border-line bg-surface px-2 py-2 text-ink"
          onChange={(event) => props.onRange(event.target.value as TraceRange)}
          value={props.filters.range}
        >
          <option value="24h">24 hours</option>
          <option value="7d">7 days</option>
          <option value="30d">30 days</option>
          <option value="custom">Custom</option>
        </select>
      </label>
      {props.filters.range === "custom" && (
        <CustomRange filters={props.filters} onFilters={props.onFilters} />
      )}
    </div>
  );
}

function CustomRange({ filters, onFilters }: Pick<TraceTableProps, "filters" | "onFilters">) {
  return (
    <>
      <label>
        Start
        <input
          className="ml-2 border border-line bg-surface px-2 py-2 text-ink"
          onChange={(event) => updateCustomTimestamp(event.target.value, "start", onFilters)}
          type="datetime-local"
          value={formatLocalDateTime(filters.start)}
        />
      </label>
      <label>
        End
        <input
          className="ml-2 border border-line bg-surface px-2 py-2 text-ink"
          onChange={(event) => updateCustomTimestamp(event.target.value, "end", onFilters)}
          type="datetime-local"
          value={formatLocalDateTime(filters.end)}
        />
      </label>
    </>
  );
}

function TraceRows({ appId, items }: Pick<TraceTableProps, "appId" | "items">) {
  return (
    <div className="mt-6 overflow-x-auto border border-line bg-surface">
      <table className="w-full text-left text-sm">
        <thead className="border-b border-line bg-canvas text-xs uppercase tracking-wide text-muted">
          <tr>
            {["Start", "Operation", "Status", "Scores", "Duration", "Spans", "Tokens"].map(
              (heading) => (
                <th className="px-4 py-3" key={heading}>
                  {heading}
                </th>
              ),
            )}
          </tr>
        </thead>
        <tbody>
          {items.map((trace) => (
            <tr className="border-b border-line last:border-0" key={trace.id}>
              <td className="whitespace-nowrap px-4 py-3">{formatStart(trace.start_time)}</td>
              <td className="px-4 py-3">
                <Link
                  className="font-medium text-accent hover:underline"
                  to={`/apps/${appId}/traces/${trace.id}`}
                >
                  {trace.root_name}
                </Link>
              </td>
              <td className="px-4 py-3 capitalize">{trace.status}</td>
              <td className="px-4 py-3">
                <ScoreBadges trace={trace} />
              </td>
              <td className="px-4 py-3 font-mono text-xs">{traceDuration(trace)}</td>
              <td className="px-4 py-3">{trace.span_count}</td>
              <td className="px-4 py-3">{trace.total_tokens}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ScoreBadges({ trace }: { trace: TraceListResponse }) {
  const summaries = trace.score_summaries ?? [];
  if (summaries.length === 0) return <span className="text-muted">Not scored</span>;
  return (
    <div className="flex flex-wrap gap-1">
      {summaries.map((summary) => (
        <Link
          aria-label={`View ${summary.scorer} score evidence`}
          className={
            summary.passed
              ? "bg-emerald-100 px-2 py-1 text-emerald-900"
              : "bg-rose-100 px-2 py-1 text-rose-900"
          }
          key={summary.scorer}
          to={`/apps/${trace.application_id}/traces/${trace.id}?tab=scores&scorer=${summary.scorer}`}
        >
          {summary.scorer} {summary.value} {summary.passed ? "pass" : "fail"}
        </Link>
      ))}
    </div>
  );
}

function updateCustomTimestamp(
  value: string,
  field: "start" | "end",
  onFilters: TraceTableProps["onFilters"],
): void {
  const timestamp = parseLocalDateTime(value);
  if (timestamp !== undefined) onFilters({ [field]: timestamp });
}

function traceQuery(appId: string, filters: TraceFilters): ListTracesQuery {
  return {
    application_id: appId,
    end: filters.end,
    ...(filters.scorer === undefined || filters.passed === undefined
      ? {}
      : { passed: String(filters.passed) as "true" | "false" }),
    ...(filters.q === "" ? {} : { q: filters.q }),
    ...(filters.scorer === undefined ? {} : { scorer: filters.scorer }),
    start: filters.start,
    ...(filters.status === "" ? {} : { status: filters.status }),
  };
}

function uniqueTraces(items: TraceListResponse[]): TraceListResponse[] {
  return [...new Map(items.map((trace) => [trace.id, trace])).values()];
}

function errorMessage(reason: unknown, fallback: string): string {
  return reason instanceof Problem ? (reason.detail ?? reason.title) : fallback;
}

function formatStart(value: string): string {
  return new Date(value).toLocaleString();
}

function traceDuration(trace: TraceListResponse): string {
  return `${Math.max(0, new Date(trace.end_time).getTime() - new Date(trace.start_time).getTime())} ms`;
}
