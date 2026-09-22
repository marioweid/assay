import { useEffect, useRef, useState } from "react";

import { Problem } from "@/api/errors";
import { listEvalRunItems } from "@/api/generated/sdk.gen";
import { Button } from "@/components/ui/button";

const MAX_ITEMS = 10_000;
const MAX_BYTES = 50 * 1024 * 1024;

export async function collectRunResultsJSONL(runID: string, signal?: AbortSignal): Promise<string> {
  const lines: string[] = [];
  const cursors = new Set<string>();
  const encoder = new TextEncoder();
  let cursor: string | undefined;
  let bytes = 0;
  let complete = false;

  while (!complete) {
    const response = await listEvalRunItems({
      path: { id: runID },
      query: { limit: 100, ...(cursor === undefined ? {} : { cursor }) },
      ...(signal === undefined ? {} : { signal }),
      throwOnError: true,
    });
    for (const item of response.data.items ?? []) {
      if (lines.length === MAX_ITEMS) {
        throw new Error("Export exceeds 10,000 cases. Use the CLI for larger runs.");
      }
      const line = `${JSON.stringify(item)}\n`;
      bytes += encoder.encode(line).byteLength;
      if (bytes > MAX_BYTES) {
        throw new Error("Export exceeds 50 MiB. Use the CLI for larger runs.");
      }
      lines.push(line);
    }
    const nextCursor = response.data.next_cursor;
    if (nextCursor === undefined) {
      complete = true;
    } else {
      if (cursors.has(nextCursor))
        throw new Error("Export stopped because the server repeated a cursor.");
      cursors.add(nextCursor);
      cursor = nextCursor;
    }
  }

  return lines.join("");
}

export function ExportRunAction({ runID }: { runID: string }) {
  const request = useRef<AbortController | null>(null);
  const [exporting, setExporting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => () => request.current?.abort(), []);

  async function download(): Promise<void> {
    const controller = new AbortController();
    request.current = controller;
    setExporting(true);
    setError(null);
    try {
      const jsonl = await collectRunResultsJSONL(runID, controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(new Blob([jsonl], { type: "application/x-ndjson" }));
      const link = document.createElement("a");
      link.href = url;
      link.download = `evaluation-run-${runID}.jsonl`;
      link.click();
      URL.revokeObjectURL(url);
    } catch (reason) {
      if (!controller.signal.aborted) {
        setError(
          reason instanceof Problem
            ? (reason.detail ?? reason.title)
            : reason instanceof Error
              ? reason.message
              : "Unable to export run",
        );
      }
    } finally {
      if (!controller.signal.aborted) setExporting(false);
    }
  }

  return (
    <div>
      <Button disabled={exporting} onClick={() => void download()}>
        {exporting ? "Exporting..." : "Export JSONL"}
      </Button>
      {error !== null && (
        <p className="mt-2 max-w-xs text-sm text-danger" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
