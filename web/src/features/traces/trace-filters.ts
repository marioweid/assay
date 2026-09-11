export const traceRanges = ["24h", "7d", "30d", "custom"] as const;

export type TraceRange = (typeof traceRanges)[number];
export type TraceScorer = "groundedness" | "correctness";

export type TraceFilters = {
  end: string;
  passed?: boolean | undefined;
  q: string;
  range: TraceRange;
  scorer?: TraceScorer | undefined;
  start: string;
  status: string;
};

export function readTraceFilters(params: URLSearchParams, now: Date): TraceFilters {
  const selectedRange = params.get("range");
  const range = isTraceRange(selectedRange) ? selectedRange : "24h";
  const fallback = relativeTraceFilters(range, now);
  const scorer = params.get("scorer");
  const passed = params.get("passed");
  const validScorer = scorer === "groundedness" || scorer === "correctness" ? scorer : undefined;
  return {
    end: validTimestamp(params.get("end")) ? params.get("end")! : fallback.end,
    ...(validScorer !== undefined && (passed === "true" || passed === "false")
      ? { passed: passed === "true" }
      : {}),
    q: params.get("q") ?? "",
    range,
    ...(validScorer === undefined ? {} : { scorer: validScorer }),
    start: validTimestamp(params.get("start")) ? params.get("start")! : fallback.start,
    status: params.get("status") ?? "",
  };
}

export function relativeTraceFilters(
  range: TraceRange,
  now: Date,
): Pick<TraceFilters, "start" | "end"> {
  const end = now.toISOString();
  const hours = range === "7d" ? 7 * 24 : range === "30d" ? 30 * 24 : 24;
  return { end, start: new Date(now.getTime() - hours * 60 * 60 * 1000).toISOString() };
}

export function formatLocalDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return [date.getFullYear(), date.getMonth() + 1, date.getDate()]
    .map((part, index) => (index === 0 ? String(part) : String(part).padStart(2, "0")))
    .join("-")
    .concat(
      "T",
      String(date.getHours()).padStart(2, "0"),
      ":",
      String(date.getMinutes()).padStart(2, "0"),
    );
}

export function parseLocalDateTime(value: string): string | undefined {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(value);
  if (match === null) return undefined;
  const [, year, month, day, hour, minute] = match;
  const date = new Date(Number(year), Number(month) - 1, Number(day), Number(hour), Number(minute));
  if (
    date.getFullYear() !== Number(year) ||
    date.getMonth() !== Number(month) - 1 ||
    date.getDate() !== Number(day) ||
    date.getHours() !== Number(hour) ||
    date.getMinutes() !== Number(minute)
  ) {
    return undefined;
  }
  return date.toISOString();
}

export function serializeTraceFilters(filters: TraceFilters): string {
  const params = new URLSearchParams({
    end: filters.end,
    range: filters.range,
    start: filters.start,
  });
  if (filters.q !== "") params.set("q", filters.q);
  if (filters.status !== "") params.set("status", filters.status);
  if (filters.scorer !== undefined) params.set("scorer", filters.scorer);
  if (filters.scorer !== undefined && filters.passed !== undefined) {
    params.set("passed", String(filters.passed));
  }
  return `?${params.toString()}`;
}

function isTraceRange(value: string | null): value is TraceRange {
  return value !== null && traceRanges.includes(value as TraceRange);
}

function validTimestamp(value: string | null): value is string {
  return value !== null && !Number.isNaN(new Date(value).getTime());
}
