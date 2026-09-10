import { useState } from "react";

import type { SpanKey, ConversationCall } from "@/features/traces/conversation-model";
import { JsonView } from "@/components/json-view";
import { MessageContent } from "@/features/traces/message-content";

export function ConversationView({
  calls,
  selectedSpanKey,
  onSelectSpan,
}: {
  calls: readonly ConversationCall[];
  selectedSpanKey: SpanKey | null;
  onSelectSpan: (key: SpanKey | null) => void;
}) {
  const visibleCalls =
    selectedSpanKey === null ? calls : calls.filter((call) => call.spanKey === selectedSpanKey);
  if (visibleCalls.length === 0) return <p className="text-sm text-muted">Content not captured</p>;
  const scorableCalls = visibleCalls.filter((call) => call.span.is_scorable);
  const expandedKey =
    selectedSpanKey ??
    (scorableCalls.length === 1 ? scorableCalls[0]?.spanKey : visibleCalls[0]?.spanKey);
  return (
    <section aria-label="Conversation" className="space-y-4">
      {visibleCalls.map((call) => (
        <ConversationCallView
          call={call}
          initiallyExpanded={call.spanKey === expandedKey}
          key={call.spanKey}
          onSelectSpan={onSelectSpan}
        />
      ))}
    </section>
  );
}

function ConversationCallView({
  call,
  initiallyExpanded,
  onSelectSpan,
}: {
  call: ConversationCall;
  initiallyExpanded: boolean;
  onSelectSpan: (key: SpanKey) => void;
}) {
  const [expanded, setExpanded] = useState(initiallyExpanded);
  const messages = [...call.input.messages, ...call.output.messages];
  const malformed = call.input.state === "malformed" || call.output.state === "malformed";
  return (
    <details
      className="border border-line bg-surface"
      onToggle={(event) => setExpanded(event.currentTarget.open)}
      open={expanded}
    >
      <summary className="flex cursor-pointer flex-wrap items-center justify-between gap-2 p-4">
        <strong>{call.span.name}</strong>
        <span className="text-xs text-muted">
          {call.span.input_tokens} input · {call.span.output_tokens} output tokens
        </span>
      </summary>
      <div className="space-y-3 px-4 pb-4">
        {messages.map((message) => (
          <div className="space-y-1" key={message.key}>
            <MessageContent message={message} />
            <button className="text-xs text-accent" onClick={() => onSelectSpan(call.spanKey)}>
              View source for {message.role} message
            </button>
          </div>
        ))}
        {call.input.state === "empty" && call.output.state === "empty" && (
          <p className="text-sm text-muted">Captured empty messages</p>
        )}
        {malformed && <MalformedCapture raw={{ input: call.input.raw, output: call.output.raw }} />}
      </div>
    </details>
  );
}

function MalformedCapture({ raw }: { raw: unknown }) {
  return (
    <details className="border border-warning p-3">
      <summary className="cursor-pointer text-sm">Could not parse captured messages</summary>
      <div className="mt-2">
        <JsonView value={raw} />
      </div>
    </details>
  );
}
