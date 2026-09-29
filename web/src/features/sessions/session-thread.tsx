import { Link } from "react-router";

import type { SessionTurnResponse } from "@/api/generated/types.gen";
import { normalizeMessages, type ConversationMessage } from "@/features/traces/conversation-model";
import { ToolActivity } from "@/features/traces/tool-activity";

export function capturedTurn(turn: SessionTurnResponse) {
  const input = normalizeMessages(turn.attributes["gen_ai.input.messages"], "input");
  const output = normalizeMessages(turn.attributes["gen_ai.output.messages"], "output");
  return {
    messages: [...input.messages, ...output.messages].filter(
      (message) => message.role === "user" || message.role === "assistant",
    ),
    malformed: input.state === "malformed" || output.state === "malformed",
  };
}

export function turnDuration(start: string, end: string): string {
  const milliseconds = Date.parse(end) - Date.parse(start);
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return "Timing unavailable";
  return milliseconds < 1000
    ? `${milliseconds.toLocaleString()} ms`
    : `${(milliseconds / 1000).toLocaleString(undefined, { maximumFractionDigits: 2 })}s`;
}

export function SessionSummary({
  turns,
  hasMore,
  sessionId,
}: {
  turns: SessionTurnResponse[];
  hasMore: boolean;
  sessionId: string;
}) {
  const last = turns.at(-1);
  const messages = turns.reduce((count, turn) => count + capturedTurn(turn).messages.length, 0);
  return (
    <section aria-label="Session summary" className="session-summary">
      <span>
        <strong>{turns.length}</strong> {hasMore ? "loaded " : ""}
        {turns.length === 1 ? "turn" : "turns"}
      </span>
      <span>
        <strong>{messages}</strong> {hasMore ? "loaded " : ""}
        {messages === 1 ? "message" : "messages"}
      </span>
      {last !== undefined && (
        <span>
          <strong>{turnDuration(last.start_time, last.end_time)}</strong>{" "}
          {hasMore ? "last loaded turn" : "last turn"}
        </span>
      )}
      <span className="session-summary-id">
        Session <strong title={sessionId}>{sessionId}</strong>
      </span>
    </section>
  );
}

export function SessionThread({
  appId,
  turns,
  selectedId,
  onSelect,
}: {
  appId: string;
  turns: SessionTurnResponse[];
  selectedId: string | null;
  onSelect: (id: string) => void;
}) {
  return (
    <ol className="session-transcript" aria-label="Session turns">
      {turns.map((turn, index) => (
        <li className="session-turn" key={turn.id}>
          <TurnMessages
            appId={appId}
            turn={turn}
            index={index + 1}
            selected={selectedId === turn.id}
            onSelect={onSelect}
          />
        </li>
      ))}
    </ol>
  );
}

function TurnMessages({
  appId,
  turn,
  index,
  selected,
  onSelect,
}: {
  appId: string;
  turn: SessionTurnResponse;
  index: number;
  selected: boolean;
  onSelect: (id: string) => void;
}) {
  const { messages, malformed } = capturedTurn(turn);
  return (
    <>
      {messages.map((message) => (
        <ChatMessage
          key={message.key}
          message={message}
          turn={turn}
          index={index}
          selected={selected}
          onSelect={onSelect}
        />
      ))}
      {messages.length === 0 && (
        <div className="session-missing-capture">
          <p>
            {malformed
              ? "Captured messages could not be parsed. Open the trace for raw evidence."
              : "No user or assistant messages captured here. Open the trace for more detail."}
          </p>
          <button className="session-control" onClick={() => onSelect(turn.id)} type="button">
            Inspect source trace
          </button>
        </div>
      )}
      {turn.status === "error" && (
        <p className="text-sm text-danger">This turn ended with an error.</p>
      )}
      {messages.length === 0 && (
        <Link className="session-trace-link" to={`/apps/${appId}/traces/${turn.id}`}>
          <span className="sr-only">Turn {index} · </span>Full trace →
        </Link>
      )}
    </>
  );
}

function ChatMessage({
  message,
  turn,
  index,
  selected,
  onSelect,
}: {
  message: ConversationMessage;
  turn: SessionTurnResponse;
  index: number;
  selected: boolean;
  onSelect: (id: string) => void;
}) {
  const text = message.parts.filter((part) => part.kind === "text");
  const evidence = message.parts.filter((part) => part.kind !== "text");
  return (
    <div className="session-message" data-speaker={message.role}>
      <span className="session-speaker">
        {message.role === "user" ? "You" : "Assistant"} ·{" "}
        <time dateTime={turn.start_time} title={new Date(turn.start_time).toLocaleString()}>
          {new Date(turn.start_time).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
        </time>
      </span>
      <button
        aria-pressed={selected}
        className="session-bubble"
        onClick={() => onSelect(turn.id)}
        type="button"
      >
        <span className="sr-only">Inspect source for {message.role} message: </span>
        {text.length > 0 ? (
          text.map((part, partIndex) => (
            <span className="block whitespace-pre-wrap" key={partIndex}>
              {part.text}
            </span>
          ))
        ) : (
          <span>View captured {message.role} message</span>
        )}
      </button>
      {evidence.length > 0 && (
        <details className="session-message-evidence">
          <summary>Message tools and attachments</summary>
          {evidence.map((part, partIndex) => (
            <ToolActivity key={partIndex} part={part} />
          ))}
        </details>
      )}
      <span className="session-turn-meta">
        {message.role === "user"
          ? `Turn ${String(index).padStart(2, "0")} · select to inspect ↗`
          : `${turnDuration(turn.start_time, turn.end_time)} · ${turn.total_tokens.toLocaleString()} tokens`}
      </span>
    </div>
  );
}
