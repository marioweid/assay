import type { SpanResponse } from "@/api/generated/types.gen";

export type SpanKey = `${string}@${string}`;
export type CaptureState = "missing" | "empty" | "present" | "malformed";
export type ConversationPart =
  | { kind: "text"; text: string }
  | { kind: "tool-call"; id: string | null; name: string; arguments: unknown }
  | { kind: "tool-result"; id: string | null; result: unknown }
  | { kind: "unsupported"; type: string; raw: unknown };
export type ConversationMessage = {
  key: string;
  role: string;
  direction: "input" | "output";
  parts: ConversationPart[];
  raw: unknown;
};
export type CapturedMessages = {
  state: CaptureState;
  messages: ConversationMessage[];
  diagnostics: string[];
  raw: unknown;
};
export type ConversationCall = {
  spanKey: SpanKey;
  span: SpanResponse;
  input: CapturedMessages;
  output: CapturedMessages;
};

type RecordValue = Record<string, unknown>;

export function spanKey(span: SpanResponse): SpanKey {
  return `${span.otel_span_id}@${span.start_time}`;
}

export function normalizeMessages(value: unknown, direction: "input" | "output"): CapturedMessages {
  if (value === undefined) return captured("missing", [], [], value);

  const values = messageValues(value);
  if (values === null) return captured("malformed", [], ["Messages must be a JSON array."], value);
  if (values.length === 0) return captured("empty", [], [], value);

  const diagnostics: string[] = [];
  const messages: ConversationMessage[] = [];
  for (const [index, item] of values.entries()) {
    const message = normalizeMessage(item, direction, index, diagnostics);
    if (message !== null) messages.push(message);
  }
  return captured(messages.length === 0 ? "malformed" : "present", messages, diagnostics, value);
}

export function conversationCalls(spans: readonly SpanResponse[]): ConversationCall[] {
  const calls: ConversationCall[] = [];
  for (const span of flattenedSpans(spans)) {
    if (!hasMessages(span)) continue;
    const key = spanKey(span);
    calls.push({
      spanKey: key,
      span,
      input: withSpanKey(normalizeMessages(span.attributes["gen_ai.input.messages"], "input"), key),
      output: withSpanKey(
        normalizeMessages(span.attributes["gen_ai.output.messages"], "output"),
        key,
      ),
    });
  }
  return calls.sort(compareCalls);
}

function captured(
  state: CaptureState,
  messages: ConversationMessage[],
  diagnostics: string[],
  raw: unknown,
): CapturedMessages {
  return { state, messages, diagnostics, raw };
}

function messageValues(value: unknown): unknown[] | null {
  if (Array.isArray(value)) return value;
  if (typeof value !== "string") return null;
  try {
    const parsed: unknown = JSON.parse(value);
    return Array.isArray(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

function normalizeMessage(
  value: unknown,
  direction: ConversationMessage["direction"],
  index: number,
  diagnostics: string[],
): ConversationMessage | null {
  if (!isRecord(value) || typeof value["role"] !== "string") {
    diagnostics.push(`Message ${index + 1} has no string role.`);
    return null;
  }

  const parts = messageParts(value, index, diagnostics);
  return { key: `${direction}:${index}`, role: value["role"], direction, parts, raw: value };
}

function messageParts(
  value: RecordValue,
  index: number,
  diagnostics: string[],
): ConversationPart[] {
  if (Array.isArray(value["parts"])) {
    return value["parts"].flatMap((part, partIndex) =>
      normalizePart(part, index, partIndex, diagnostics),
    );
  }
  if (Object.hasOwn(value, "parts")) {
    diagnostics.push(`Message ${index + 1} has invalid parts.`);
    return [];
  }
  if (typeof value["content"] === "string") return [{ kind: "text", text: value["content"] }];
  diagnostics.push(`Message ${index + 1} has no content or parts.`);
  return [];
}

function normalizePart(
  value: unknown,
  messageIndex: number,
  partIndex: number,
  diagnostics: string[],
): ConversationPart[] {
  if (!isRecord(value) || typeof value["type"] !== "string") {
    diagnostics.push(`Message ${messageIndex + 1} part ${partIndex + 1} has no string type.`);
    return [];
  }
  if (value["type"] === "text") {
    if (typeof value["content"] === "string") return [{ kind: "text", text: value["content"] }];
    diagnostics.push(`Message ${messageIndex + 1} part ${partIndex + 1} has invalid text.`);
    return [];
  }
  if (value["type"] === "tool_call") {
    if (typeof value["name"] === "string") {
      return [
        {
          kind: "tool-call",
          id: stringOrNull(value["id"]),
          name: value["name"],
          arguments: value["arguments"],
        },
      ];
    }
    diagnostics.push(`Message ${messageIndex + 1} part ${partIndex + 1} has no tool name.`);
    return [];
  }
  if (value["type"] === "tool_call_response") {
    return [{ kind: "tool-result", id: stringOrNull(value["id"]), result: value["response"] }];
  }
  return [{ kind: "unsupported", type: value["type"], raw: value }];
}

function isRecord(value: unknown): value is RecordValue {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function stringOrNull(value: unknown): string | null {
  return typeof value === "string" ? value : null;
}

function flattenedSpans(spans: readonly SpanResponse[]): SpanResponse[] {
  const result: SpanResponse[] = [];
  const pending = [...spans].reverse();
  const visited = new Set<SpanResponse>();
  while (pending.length > 0) {
    const span = pending.pop();
    if (span === undefined || visited.has(span)) continue;
    visited.add(span);
    result.push(span);
    pending.push(...(span.children ?? []).slice().reverse());
  }
  return result;
}

function hasMessages(span: SpanResponse): boolean {
  return (
    Object.hasOwn(span.attributes, "gen_ai.input.messages") ||
    Object.hasOwn(span.attributes, "gen_ai.output.messages")
  );
}

function withSpanKey(messages: CapturedMessages, source: SpanKey): CapturedMessages {
  return {
    ...messages,
    messages: messages.messages.map((message) => ({ ...message, key: `${source}:${message.key}` })),
  };
}

function compareCalls(left: ConversationCall, right: ConversationCall): number {
  const leftTime = Date.parse(left.span.start_time);
  const rightTime = Date.parse(right.span.start_time);
  if (Number.isFinite(leftTime) && Number.isFinite(rightTime) && leftTime !== rightTime) {
    return leftTime - rightTime;
  }
  if (Number.isFinite(leftTime) !== Number.isFinite(rightTime))
    return Number.isFinite(leftTime) ? -1 : 1;
  return left.spanKey.localeCompare(right.spanKey);
}
