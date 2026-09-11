import type { ConversationPart } from "@/features/traces/conversation-model";
import { JsonView } from "@/components/json-view";

export function ToolActivity({ part }: { part: Exclude<ConversationPart, { kind: "text" }> }) {
  if (part.kind === "tool-call")
    return (
      <details className="border border-line p-2">
        <summary className="cursor-pointer font-mono text-xs">Tool call: {part.name}</summary>
        <div className="mt-2">
          <JsonView value={part.arguments} />
        </div>
      </details>
    );
  if (part.kind === "tool-result")
    return (
      <details className="border border-line p-2">
        <summary className="cursor-pointer font-mono text-xs">Tool result</summary>
        <div className="mt-2">
          <JsonView value={part.result} />
        </div>
      </details>
    );
  return (
    <details className="border border-line p-2">
      <summary className="cursor-pointer font-mono text-xs">Unsupported part: {part.type}</summary>
      <div className="mt-2">
        <JsonView value={part.raw} />
      </div>
    </details>
  );
}
