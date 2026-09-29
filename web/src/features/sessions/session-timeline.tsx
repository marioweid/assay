import { Minus, Plus } from "lucide-react";
import { useState } from "react";

import type { SessionTurnResponse } from "@/api/generated/types.gen";
import { layoutSpanTiming } from "@/features/traces/span-timing";

export function SessionTimeline({
  turns,
  selectedId,
  onSelect,
}: {
  turns: SessionTurnResponse[];
  selectedId: string | null;
  onSelect: (id: string) => void;
}) {
  const [zoom, setZoom] = useState(1);
  const layout = layoutSpanTiming(
    turns.map((turn) => ({
      key: `${turn.id}@${turn.start_time}`,
      startTime: turn.start_time,
      endTime: turn.end_time,
    })),
  );
  return (
    <section aria-label="Session timeline" className="session-timeline">
      <TimelineToolbar durationMs={layout.durationMs} onZoom={setZoom} zoom={zoom} />
      <div
        aria-label="Scrollable timing chart"
        className="mt-4 overflow-x-auto"
        role="region"
        tabIndex={0}
      >
        <div data-testid="timeline-content" style={{ minWidth: `${34 * zoom}rem` }}>
          <div
            className="mb-2 ml-[9rem] flex justify-between text-xs text-muted"
            aria-hidden="true"
          >
            <span>0 ms</span>
            <span>{formatMs(layout.durationMs / 2)} ms</span>
            <span>{formatMs(layout.durationMs)} ms</span>
          </div>
          <ol className="space-y-2">
            {turns.map((turn, index) => {
              const timing = layout.rows[index];
              if (timing === undefined) return null;
              const summary = timing.invalid
                ? "Invalid timing"
                : `${formatMs(timing.offsetMs)} ms start, ${formatMs(timing.durationMs)} ms duration`;
              return (
                <li key={turn.id}>
                  <button
                    aria-pressed={selectedId === turn.id}
                    className="session-timeline-row"
                    onClick={() => onSelect(turn.id)}
                    type="button"
                  >
                    <span className="min-w-0">
                      <span className="block truncate text-sm">
                        Turn {index + 1} · {turn.root_name}
                      </span>
                      <span className="block text-xs text-muted">{summary}</span>
                    </span>
                    <span
                      className="relative h-7 border-x border-line bg-canvas"
                      aria-hidden="true"
                    >
                      {!timing.invalid && (
                        <span
                          className="absolute top-1 h-5 min-w-0.5 rounded bg-accent"
                          style={{
                            left: `${timing.leftPercent}%`,
                            width: `${Math.max(timing.widthPercent, 0.3)}%`,
                          }}
                        />
                      )}
                    </span>
                  </button>
                </li>
              );
            })}
          </ol>
        </div>
      </div>
      <p className="mt-4 text-xs text-muted">
        Scroll horizontally to explore long sessions. Select a row for its source trace.
      </p>
    </section>
  );
}

function TimelineToolbar({
  durationMs,
  onZoom,
  zoom,
}: {
  durationMs: number;
  onZoom: (value: number) => void;
  zoom: number;
}) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <p className="text-xs text-muted">Loaded turns · {formatMs(durationMs)} ms total</p>
      <div aria-label="Timeline zoom" className="flex items-center gap-1">
        <button
          aria-label="Zoom out"
          className="session-icon-button"
          disabled={zoom === 1}
          onClick={() => onZoom(zoom / 2)}
          type="button"
        >
          <Minus aria-hidden="true" size={16} />
        </button>
        <span aria-live="polite" className="min-w-7 text-center text-xs">
          {zoom}×
        </span>
        <button
          aria-label="Zoom in"
          className="session-icon-button"
          disabled={zoom === 4}
          onClick={() => onZoom(zoom * 2)}
          type="button"
        >
          <Plus aria-hidden="true" size={16} />
        </button>
      </div>
    </div>
  );
}

function formatMs(value: number): string {
  return value.toLocaleString(undefined, { maximumFractionDigits: 3 });
}
