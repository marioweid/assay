import type { SpanResponse } from "@/api/generated/types.gen";
import { JsonView } from "@/components/json-view";

export function RetrievalContext({ spans }: { spans: readonly SpanResponse[] }) {
  const standardDocuments = spans.flatMap(documentsForSpan);
  const documents =
    standardDocuments.length > 0 ? standardDocuments : spans.flatMap(flattenedChunks);
  if (documents.length === 0) return null;
  return (
    <section aria-labelledby="retrieval-context-heading" className="space-y-2">
      <h3 className="font-semibold" id="retrieval-context-heading">
        Retrieved context
      </h3>
      {documents.map((document, index) => (
        <details className="border border-line p-3" key={index}>
          <summary className="cursor-pointer text-sm">Document {index + 1}</summary>
          <div className="mt-2">
            <JsonView value={document} />
          </div>
        </details>
      ))}
    </section>
  );
}

function flattenedChunks(span: SpanResponse): unknown[] {
  const count = span.attributes["assay.context.chunk.count"];
  if (!Number.isInteger(count) || typeof count !== "number" || count < 0) return [];
  const chunks: Array<{ id: string; text: string }> = [];
  for (let index = 0; index < count; index += 1) {
    const prefix = `assay.context.chunks.${index}`;
    const id = span.attributes[`${prefix}.id`];
    const text = span.attributes[`${prefix}.text`];
    if (typeof id === "string" && typeof text === "string") chunks.push({ id, text });
  }
  return chunks;
}

function documentsForSpan(span: SpanResponse): unknown[] {
  const value = span.attributes["gen_ai.retrieval.documents"];
  if (Array.isArray(value)) return value;
  if (typeof value !== "string") return [];
  try {
    const parsed: unknown = JSON.parse(value);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}
