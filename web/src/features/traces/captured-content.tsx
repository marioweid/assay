import type { SpanResponse } from "@/api/generated/types.gen";

const fields = [
  ["Input", "gen_ai.input.messages"],
  ["Output", "gen_ai.output.messages"],
  ["Context", "gen_ai.retrieval.documents"],
] as const;

export function CapturedContent({ spans }: { spans: SpanResponse[] }) {
  return (
    <div className="mt-6 space-y-6">
      {spans.map((span) => (
        <section className="space-y-3" key={span.id} aria-label={`Captured content: ${span.name}`}>
          <h3 className="font-semibold">{span.name}</h3>
          <p className="text-xs text-muted">
            <span>{String(span.attributes["gen_ai.request.model"] ?? "")}</span>
            {` · ${span.input_tokens} input / ${span.output_tokens} output tokens`}
          </p>
          {fields.map(([label, key]) =>
            span.attributes[key] === undefined ? null : (
              <div key={key}>
                <h4 className="mb-2 text-xs uppercase tracking-wide text-muted">{label}</h4>
                <ContentValue value={decode(span.attributes[key])} />
              </div>
            ),
          )}
        </section>
      ))}
      {spans.length === 0 && <p className="text-sm text-muted">No input or output captured.</p>}
    </div>
  );
}

export function capturedSpans(spans: SpanResponse[]): SpanResponse[] {
  const result: SpanResponse[] = [];
  for (const span of spans) {
    if (fields.some(([, key]) => span.attributes[key] !== undefined)) result.push(span);
    result.push(...capturedSpans(span.children ?? []));
  }
  return result;
}

function decode(value: unknown): unknown {
  if (typeof value !== "string") return value;
  try {
    return JSON.parse(value) as unknown;
  } catch {
    return value;
  }
}

function ContentValue({ value }: { value: unknown }) {
  if (Array.isArray(value))
    return (
      <div className="space-y-2">
        {value.map((item: unknown, index) => (
          <ContentValue key={index} value={item} />
        ))}
      </div>
    );
  if (value !== null && typeof value === "object") {
    const record = value as Record<string, unknown>;
    const content = record["content"] ?? record["text"];
    if (content !== undefined)
      return (
        <div className="border border-line bg-surface p-3">
          <p className="mb-1 text-xs text-muted">{String(record["role"] ?? record["id"] ?? "")}</p>
          <ContentValue value={content} />
        </div>
      );
  }
  return (
    <pre className="whitespace-pre-wrap break-words font-sans text-sm">
      {typeof value === "string" ? value : JSON.stringify(value, null, 2)}
    </pre>
  );
}
