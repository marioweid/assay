import type { ConversationMessage } from "@/features/traces/conversation-model";
import { ToolActivity } from "@/features/traces/tool-activity";

export function MessageContent({ message }: { message: ConversationMessage }) {
  return (
    <article className="rounded border border-line bg-canvas p-3">
      <div className="mb-2 flex items-center justify-between gap-3">
        <strong className="text-sm capitalize">{message.role}</strong>
      </div>
      <div className="space-y-2">
        {message.parts.map((part, index) => {
          if (part.kind === "text")
            return (
              <pre className="whitespace-pre-wrap break-words font-sans text-sm" key={index}>
                {part.text}
              </pre>
            );
          return <ToolActivity key={index} part={part} />;
        })}
      </div>
    </article>
  );
}
