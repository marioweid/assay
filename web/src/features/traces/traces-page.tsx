import { useDeferredValue, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";

import { Problem } from "@/api/errors";
import { listTraces } from "@/api/generated/sdk.gen";
import type { TraceResponse } from "@/api/generated/types.gen";

export function TracesPage() {
  const { appId = "" } = useParams();
  const requestNumber = useRef(0);
  const activeRequest = useRef<AbortController | null>(null);
  const [items, setItems] = useState<TraceResponse[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [seenCursors, setSeenCursors] = useState<Set<string>>(new Set());
  const [filter, setFilter] = useState("");
  const deferredFilter = useDeferredValue(filter.trim().toLowerCase());
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
      query: { application_id: appId },
      signal: controller.signal,
      throwOnError: true,
    })
      .then((response) => {
        if (requestNumber.current !== currentRequest || controller.signal.aborted) return;
        setItems(response.data.items ?? []);
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
    return () => {
      controller.abort();
      activeRequest.current?.abort();
    };
  }, [appId]);

  async function loadMore(): Promise<void> {
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
        query: { application_id: appId, cursor },
        signal: controller.signal,
        throwOnError: true,
      });
      if (requestNumber.current !== currentRequest || controller.signal.aborted) return;
      const receivedCursor = response.data.next_cursor ?? null;
      setItems((current) => [...current, ...(response.data.items ?? [])]);
      setSeenCursors(consumed);
      if (receivedCursor !== null && consumed.has(receivedCursor)) {
        setError("The server returned a repeated cursor. Pagination stopped.");
        setNextCursor(null);
      } else setNextCursor(receivedCursor);
    } catch (reason) {
      if (!controller.signal.aborted) setError(errorMessage(reason, "Unable to load more traces"));
    } finally {
      if (requestNumber.current === currentRequest) setLoading(false);
    }
  }

  const visibleItems = items.filter(
    (trace) =>
      deferredFilter === "" ||
      trace.root_name.toLowerCase().includes(deferredFilter) ||
      trace.status.toLowerCase().includes(deferredFilter),
  );
  return (
    <TraceTable
      appId={appId}
      error={error}
      filter={filter}
      items={visibleItems}
      loading={loading}
      nextCursor={nextCursor}
      onFilter={setFilter}
      onLoadMore={loadMore}
    />
  );
}

type TraceTableProps = {
  appId: string;
  error: string | null;
  filter: string;
  items: TraceResponse[];
  loading: boolean;
  nextCursor: string | null;
  onFilter: (value: string) => void;
  onLoadMore: () => Promise<void>;
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
        <label className="text-sm text-muted">
          Filter traces
          <input
            className="ml-2 border border-line bg-surface px-3 py-2 text-ink"
            onChange={(event) => props.onFilter(event.target.value)}
            value={props.filter}
          />
        </label>
      </div>
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
      {props.items.length > 0 && (
        <div className="mt-6 overflow-x-auto border border-line bg-surface">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-line bg-canvas text-xs uppercase tracking-wide text-muted">
              <tr>
                {["Start", "Operation", "Status", "Duration", "Spans", "Tokens"].map((heading) => (
                  <th className="px-4 py-3" key={heading}>
                    {heading}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {props.items.map((trace, index) => (
                <tr className="border-b border-line last:border-0" key={`${trace.id}-${index}`}>
                  <td className="whitespace-nowrap px-4 py-3">{formatStart(trace.start_time)}</td>
                  <td className="px-4 py-3">
                    <Link
                      className="font-medium text-accent hover:underline"
                      to={`/apps/${props.appId}/traces/${trace.id}`}
                    >
                      {trace.root_name}
                    </Link>
                  </td>
                  <td className="px-4 py-3 capitalize">{trace.status}</td>
                  <td className="px-4 py-3 font-mono text-xs">{traceDuration(trace)}</td>
                  <td className="px-4 py-3">{trace.span_count}</td>
                  <td className="px-4 py-3">{trace.total_tokens}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {props.nextCursor !== null && (
        <button
          className="mt-4 border border-line bg-surface px-4 py-2 text-sm font-medium disabled:opacity-50"
          disabled={props.loading}
          onClick={() => void props.onLoadMore()}
        >
          Load more
        </button>
      )}
    </section>
  );
}

function errorMessage(reason: unknown, fallback: string): string {
  return reason instanceof Problem ? (reason.detail ?? reason.title) : fallback;
}
function formatStart(value: string): string {
  return new Date(value).toLocaleString();
}
function traceDuration(trace: TraceResponse): string {
  return `${Math.max(0, new Date(trace.end_time).getTime() - new Date(trace.start_time).getTime())} ms`;
}
