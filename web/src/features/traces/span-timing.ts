import type { SpanKey } from "@/features/traces/conversation-model";

export type SpanTimingInput = { key: SpanKey; startTime: string; endTime: string };
export type SpanTiming = {
  key: SpanKey;
  offsetMs: number;
  durationMs: number;
  leftPercent: number;
  widthPercent: number;
  invalid: boolean;
};
export type TimingLayout = { durationMs: number; rows: SpanTiming[] };

const timestamp =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|([+-])(\d{2}):(\d{2}))$/;
const nanosecondsPerMillisecond = 1_000_000n;
const nanosecondsPerSecond = 1_000_000_000n;

export function parseTimestampNs(value: string): bigint | null {
  const match = timestamp.exec(value);
  if (match === null) return null;
  const [
    ,
    year,
    month,
    day,
    hour,
    minute,
    second,
    fraction = "",
    zone,
    sign,
    offsetHour,
    offsetMinute,
  ] = match;
  if ([year, month, day, hour, minute, second, zone].some((part) => part === undefined))
    return null;
  const values = [year, month, day, hour, minute, second].map(Number);
  if (values.some((part) => !Number.isInteger(part))) return null;
  const [parsedYear, parsedMonth, parsedDay, parsedHour, parsedMinute, parsedSecond] = values;
  if (
    parsedYear === undefined ||
    parsedMonth === undefined ||
    parsedDay === undefined ||
    parsedHour === undefined ||
    parsedMinute === undefined ||
    parsedSecond === undefined
  )
    return null;
  const localMilliseconds = Date.UTC(
    parsedYear,
    parsedMonth - 1,
    parsedDay,
    parsedHour,
    parsedMinute,
    parsedSecond,
  );
  const local = new Date(localMilliseconds);
  if (
    Number.isNaN(localMilliseconds) ||
    local.getUTCFullYear() !== parsedYear ||
    local.getUTCMonth() !== parsedMonth - 1 ||
    local.getUTCDate() !== parsedDay ||
    local.getUTCHours() !== parsedHour ||
    local.getUTCMinutes() !== parsedMinute ||
    local.getUTCSeconds() !== parsedSecond
  )
    return null;
  const parsedOffsetHour = Number(offsetHour ?? "0");
  const parsedOffsetMinute = Number(offsetMinute ?? "0");
  if (parsedOffsetHour > 23 || parsedOffsetMinute > 59) return null;
  const offsetSeconds = BigInt(parsedOffsetHour * 3600 + parsedOffsetMinute * 60);
  const signedOffset = sign === "+" ? offsetSeconds : sign === "-" ? -offsetSeconds : 0n;
  const fractionalNanoseconds = BigInt((fraction + "000000000").slice(0, 9));
  return (
    BigInt(localMilliseconds / 1000) * nanosecondsPerSecond -
    signedOffset * nanosecondsPerSecond +
    fractionalNanoseconds
  );
}

export function layoutSpanTiming(spans: readonly SpanTimingInput[]): TimingLayout {
  const parsed = spans.map((span) => ({
    ...span,
    start: parseTimestampNs(span.startTime),
    end: parseTimestampNs(span.endTime),
  }));
  const valid = parsed.filter(
    (span) => span.start !== null && span.end !== null && span.end >= span.start,
  );
  if (valid.length === 0) return { durationMs: 0, rows: spans.map(invalidTiming) };
  const origin = valid.reduce(
    (minimum, span) => (span.start! < minimum ? span.start! : minimum),
    valid[0]!.start!,
  );
  const end = valid.reduce(
    (maximum, span) => (span.end! > maximum ? span.end! : maximum),
    valid[0]!.end!,
  );
  const domain = end - origin;
  return {
    durationMs: Number(domain) / Number(nanosecondsPerMillisecond),
    rows: parsed.map((span) => timing(span, origin, domain)),
  };
}

function timing(
  span: SpanTimingInput & { start: bigint | null; end: bigint | null },
  origin: bigint,
  domain: bigint,
): SpanTiming {
  if (span.start === null || span.end === null || span.end < span.start) return invalidTiming(span);
  const offset = span.start - origin;
  const duration = span.end - span.start;
  const scale = domain === 0n ? 0 : 100 / Number(domain);
  return {
    key: span.key,
    offsetMs: Number(offset) / Number(nanosecondsPerMillisecond),
    durationMs: Number(duration) / Number(nanosecondsPerMillisecond),
    leftPercent: Math.max(0, Math.min(100, Number(offset) * scale)),
    widthPercent: Math.max(0, Math.min(100, Number(duration) * scale)),
    invalid: false,
  };
}

function invalidTiming(span: SpanTimingInput): SpanTiming {
  return {
    key: span.key,
    offsetMs: 0,
    durationMs: 0,
    leftPercent: 0,
    widthPercent: 0,
    invalid: true,
  };
}
