import { useState } from "react";
import type { KeyboardEvent } from "react";

import type { SpanResponse } from "@/api/generated/types.gen";

type SpanTreeProps = {
  selectedID: number | null;
  spans: SpanResponse[];
  onSelect: (span: SpanResponse) => void;
};

export function SpanTree({ selectedID, spans, onSelect }: SpanTreeProps) {
  const [focusID, setFocusID] = useState<number | null>(spans[0]?.id ?? null);
  return (
    <ul aria-label="Spans" role="tree">
      {spans.map((span) => (
        <SpanNode
          key={span.id}
          depth={0}
          focusID={focusID}
          onFocus={setFocusID}
          onSelect={onSelect}
          parentID={null}
          selectedID={selectedID}
          span={span}
        />
      ))}
    </ul>
  );
}

type SpanNodeProps = {
  depth: number;
  focusID: number | null;
  parentID: number | null;
  selectedID: number | null;
  span: SpanResponse;
  onFocus: (id: number) => void;
  onSelect: (span: SpanResponse) => void;
};

function SpanNode({
  depth,
  focusID,
  parentID,
  selectedID,
  span,
  onFocus,
  onSelect,
}: SpanNodeProps) {
  const children = span.children ?? [];
  const [expanded, setExpanded] = useState(true);

  function activate(): void {
    onFocus(span.id);
    onSelect(span);
    if (children.length > 0) setExpanded((value) => !value);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLButtonElement>): void {
    const tree = event.currentTarget.closest('[role="tree"]');
    if (tree === null) return;
    const items = Array.from(tree.querySelectorAll<HTMLButtonElement>('[role="treeitem"]'));
    const currentIndex = items.indexOf(event.currentTarget);
    let target: HTMLButtonElement | undefined;
    if (event.key === "ArrowDown") target = items[currentIndex + 1];
    if (event.key === "ArrowUp") target = items[currentIndex - 1];
    if (event.key === "Home") target = items[0];
    if (event.key === "End") target = items.at(-1);
    if (event.key === "ArrowRight" && children.length > 0) {
      if (!expanded) setExpanded(true);
      else target = items[currentIndex + 1];
    }
    if (event.key === "ArrowLeft") {
      if (expanded && children.length > 0) setExpanded(false);
      else target = items.find((item) => item.dataset["spanId"] === String(parentID));
    }
    if (
      target === undefined &&
      !event.key.startsWith("Arrow") &&
      event.key !== "Home" &&
      event.key !== "End"
    )
      return;
    event.preventDefault();
    if (target !== undefined) {
      onFocus(Number(target.dataset["spanId"]));
      target.focus();
    }
  }

  return (
    <li role="none">
      <button
        aria-expanded={children.length > 0 ? expanded : undefined}
        aria-level={depth + 1}
        aria-label={span.name}
        aria-owns={children.length > 0 ? `span-group-${span.id}` : undefined}
        aria-selected={selectedID === span.id}
        className={`w-full border-l-2 px-3 py-2 text-left text-sm ${selectedID === span.id ? "border-accent bg-accent/10" : "border-transparent hover:bg-canvas"}`}
        data-parent-id={parentID ?? undefined}
        data-span-id={span.id}
        onClick={activate}
        onFocus={() => onFocus(span.id)}
        onKeyDown={handleKeyDown}
        role="treeitem"
        style={{ paddingLeft: `${depth * 16 + 12}px` }}
        tabIndex={focusID === span.id ? 0 : -1}
      >
        <span className="font-medium">{span.name}</span>
        <span className="ml-2 font-mono text-xs text-muted">
          {formatDuration(span.duration_ms)}
        </span>
      </button>
      {expanded && children.length > 0 && (
        <ul id={`span-group-${span.id}`} role="group">
          {children.map((child) => (
            <SpanNode
              key={child.id}
              depth={depth + 1}
              focusID={focusID}
              onFocus={onFocus}
              onSelect={onSelect}
              parentID={span.id}
              selectedID={selectedID}
              span={child}
            />
          ))}
        </ul>
      )}
    </li>
  );
}

function formatDuration(milliseconds: number): string {
  return milliseconds < 1000
    ? `${milliseconds.toFixed(1)} ms`
    : `${(milliseconds / 1000).toFixed(2)} s`;
}
