import type { SpanResponse } from "@/api/generated/types.gen";
import { spanKey, type SpanKey } from "@/features/traces/conversation-model";
import { layoutSpanTiming } from "@/features/traces/span-timing";

export function SpanWaterfall({
  spans,
  onSelect,
}: {
  spans: readonly SpanResponse[];
  onSelect: (key: SpanKey) => void;
}) {
  const rows = flatten(spans);
  const layout = layoutSpanTiming(
    rows.map((span) => ({
      key: spanKey(span),
      startTime: span.start_time,
      endTime: span.end_time,
    })),
  );
  return (
    <section aria-label="Span timing" className="overflow-x-auto">
      <div className="min-w-[36rem] space-y-1">
        {rows.map((span, index) => {
          const timing = layout.rows[index];
          if (timing === undefined) return null;
          const duration = `${timing.durationMs.toFixed(1)} ms`;
          return (
            <button
              aria-label={`${span.name}, ${timing.invalid ? "Invalid timing" : duration}`}
              className="grid w-full grid-cols-[minmax(10rem,1fr)_minmax(12rem,2fr)] gap-3 rounded px-2 py-1 text-left hover:bg-canvas"
              key={timing.key}
              onClick={() => onSelect(timing.key)}
            >
              <span className="truncate text-sm">{span.name}</span>
              <svg
                aria-label={`Timeline for ${span.name}`}
                className="h-5 w-full"
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

function flatten(spans: readonly SpanResponse[]): SpanResponse[] {
  const result: SpanResponse[] = [];
  const pending = [...spans].reverse();
  const seen = new Set<SpanResponse>();
  while (pending.length > 0) {
    const span = pending.pop();
    if (span === undefined || seen.has(span)) continue;
    seen.add(span);
    result.push(span);
    pending.push(...(span.children ?? []).slice().reverse());
  }
  return result;
}
