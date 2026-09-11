import { useEffect, useState } from "react";
import { useParams } from "react-router";

import { applicationMetrics } from "@/api/generated/sdk.gen";
import type { MetricPoint } from "@/api/generated/types.gen";
import { EmptyState } from "@/components/empty-state";
import { LoadingState } from "@/components/loading-state";
import { ProblemState } from "@/components/problem-state";

export function MetricsPage() {
  const { appId = "" } = useParams();
  const [days, setDays] = useState("30");
  const [items, setItems] = useState<MetricPoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refresh, setRefresh] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    const end = new Date();
    const start = new Date(end.getTime() - Number(days) * 86400000);
    setItems([]);
    setError(null);
    setLoading(true);
    void applicationMetrics({
      path: { id: appId },
      query: { start: start.toISOString(), end: end.toISOString() },
      signal: controller.signal,
      throwOnError: true,
    })
      .then((response) => {
        if (!controller.signal.aborted) setItems(response.data.items ?? []);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError("Unable to load metrics. Try again later.");
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [appId, days, refresh]);

  return (
    <section>
      <div className="mb-6 flex items-center justify-between gap-4">
        <h1 className="text-2xl font-semibold">Score trends</h1>
        <select
          aria-label="Time period"
          value={days}
          onChange={(event) => setDays(event.target.value)}
          className="rounded-md border border-line bg-surface px-3 py-2 text-ink"
        >
          <option value="7">Last 7 days</option>
          <option value="30">Last 30 days</option>
          <option value="90">Last 90 days</option>
        </select>
      </div>
      <p className="mb-4 text-sm text-muted">
        Daily UTC averages across online and offline scores. Pass rates use the threshold recorded
        when each score was computed. Days without scores are omitted.
      </p>
      {loading ? <LoadingState label="Loading metrics" /> : null}
      {!loading && error !== null ? (
        <ProblemState
          detail={error}
          onRetry={() => setRefresh((current) => current + 1)}
          title="Metrics unavailable"
        />
      ) : null}
      {!loading && error === null && items.length === 0 ? (
        <EmptyState
          description="No scores were recorded in the selected period."
          title="No scores in this period"
        />
      ) : null}
      {items.length > 0 ? <TrendTable items={items} /> : null}
    </section>
  );
}

function TrendTable({ items }: { items: MetricPoint[] }) {
  return (
    <div className="overflow-x-auto border border-line bg-surface">
      <table aria-label="Daily score trends" className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-line">
            {["Date (UTC)", "Scorer", "Mean", "Pass rate", "Scores"].map((label) => (
              <th scope="col" key={label} className="px-4 py-3">
                {label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {items.map((point) => (
            <tr key={`${point.date}-${point.scorer}`}>
              <td className="px-4 py-3 font-mono">{point.date.slice(0, 10)}</td>
              <td className="px-4 py-3">{point.scorer}</td>
              <td className="min-w-36 px-4 py-3">
                <span>{(point.mean * 100).toFixed(1)}%</span>
                <div aria-hidden="true" className="mt-1 h-1.5 rounded bg-line">
                  <div
                    className="h-full rounded bg-accent"
                    style={{ width: `${point.mean * 100}%` }}
                  />
                </div>
              </td>
              <td className="px-4 py-3">{(point.pass_rate * 100).toFixed(1)}%</td>
              <td className="px-4 py-3 font-mono">{point.n}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
