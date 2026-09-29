import type { SpanResponse } from "@/api/generated/types.gen";
import { spanKey, type SpanKey } from "@/features/traces/conversation-model";
import { layoutSpanTiming } from "@/features/traces/span-timing";

export function SpanWaterfall({
  spans,
  onSelect,
  selectedKey = null,
}: {
  spans: readonly SpanResponse[];
  onSelect: (key: SpanKey) => void;
  selectedKey?: SpanKey | null;
}) {
  const rows = flatten(spans);
  const layout = layoutSpanTiming(
    rows.map(({ span }) => ({
      key: spanKey(span),
      startTime: span.start_time,
      endTime: span.end_time,
    })),
  );
  return (
    <section aria-label="Span timing" className="overflow-x-auto" tabIndex={0}>
      <div className="min-w-[16rem] space-y-1">
        <div className="ml-[7rem] flex justify-between text-xs text-muted" aria-hidden="true">
          <span>0 ms</span>
          <span>{(layout.durationMs / 2).toFixed(0)} ms</span>
          <span>{layout.durationMs.toFixed(0)} ms</span>
        </div>
        {rows.map(({ span, depth }, index) => {
          const timing = layout.rows[index];
          if (timing === undefined) return null;
          const duration = `${timing.durationMs.toFixed(1)} ms`;
          const timingLabel = timing.invalid
            ? "Invalid timing"
            : `${timing.offsetMs.toFixed(1)} ms start, ${duration}`;
          return (
            <button
              aria-label={`${span.name}, ${timingLabel}`}
              aria-pressed={selectedKey === timing.key}
              className={[
                "grid min-h-11 w-full grid-cols-[6rem_minmax(0,1fr)] items-center gap-3",
                "rounded px-1 py-2 text-left hover:bg-canvas",
                "aria-pressed:bg-accent/10 aria-pressed:text-accent",
              ].join(" ")}
              key={timing.key}
              onClick={() => onSelect(timing.key)}
              type="button"
            >
              <span
                className="truncate text-xs"
                style={{ paddingLeft: `${Math.min(depth, 5) * 12}px` }}
              >
                {span.name}
              </span>
              <svg
                aria-label={`Timeline for ${span.name}`}
                className={depth === 0 ? "h-5 w-full text-accent" : "h-5 w-full text-muted"}
                role="img"
                viewBox="0 0 100 10"
                preserveAspectRatio="none"
              >
                {timing.invalid ? (
                  <text x="0" y="8">
                    Invalid timing
                  </text>
                ) : (
                  <TimingBar left={timing.leftPercent} width={timing.widthPercent} />
                )}
              </svg>
            </button>
          );
        })}
      </div>
    </section>
  );
}

function TimingBar({ left, width }: { left: number; width: number }) {
  if (width === 0)
    return <line stroke="currentColor" strokeWidth="2" x1={left} x2={left} y1="1" y2="9" />;
  return <rect fill="currentColor" height="8" rx="1" width={width} x={left} y="1" />;
}

function flatten(spans: readonly SpanResponse[]): { span: SpanResponse; depth: number }[] {
  const result: { span: SpanResponse; depth: number }[] = [];
  const pending = spans.map((span) => ({ span, depth: 0 })).reverse();
  const seen = new Set<SpanResponse>();
  while (pending.length > 0) {
    const row = pending.pop();
    if (row === undefined || seen.has(row.span)) continue;
    seen.add(row.span);
    result.push(row);
    pending.push(
      ...(row.span.children ?? []).map((span) => ({ span, depth: row.depth + 1 })).reverse(),
    );
  }
  return result;
}
