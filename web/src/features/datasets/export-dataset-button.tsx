import { useRef, useState } from "react";

import { Problem } from "@/api/errors";
import { listDatasetItems } from "@/api/generated/sdk.gen";
import type { DatasetItemResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { serializeDatasetJsonl } from "@/features/datasets/dataset-jsonl";

const maxItems = 10_000;
const maxBytes = 50 * 1024 * 1024;

export function ExportDatasetButton({ datasetID }: { datasetID: string }): React.ReactElement {
  const [exporting, setExporting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const active = useRef<AbortController | null>(null);

  async function exportItems(): Promise<void> {
    const controller = new AbortController();
    active.current = controller;
    setExporting(true);
    setError(null);
    const items: DatasetItemResponse[] = [];
    const ids = new Set<string>();
    const cursors = new Set<string>();
    let cursor: string | undefined;
    try {
      do {
        const response = await listDatasetItems({
          path: { id: datasetID },
          query: cursor === undefined ? {} : { cursor },
          signal: controller.signal,
          throwOnError: true,
        });
        for (const item of response.data.items ?? []) {
          if (!ids.has(item.id)) {
            ids.add(item.id);
            items.push(item);
          }
        }
        if (items.length > maxItems)
          throw new Error("Exports are limited to 10,000 cases. Use the CLI.");
        cursor = response.data.next_cursor;
        if (cursor !== undefined && cursor !== null) {
          if (cursors.has(cursor))
            throw new Error("The server returned a repeated cursor. Export stopped.");
          cursors.add(cursor);
        }
      } while (cursor !== undefined && cursor !== null);
      if (controller.signal.aborted) return;
      const text = serializeDatasetJsonl(items);
      const blob = new Blob([text], { type: "application/x-ndjson" });
      if (blob.size > maxBytes) throw new Error("Exports are limited to 50 MiB. Use the CLI.");
      const link = document.createElement("a");
      link.href = URL.createObjectURL(blob);
      link.download = "dataset.jsonl";
      link.click();
      URL.revokeObjectURL(link.href);
    } catch (reason) {
      if (!controller.signal.aborted) {
        setError(reason instanceof Problem ? (reason.detail ?? reason.title) : String(reason));
      }
    } finally {
      if (active.current === controller) active.current = null;
      if (!controller.signal.aborted) setExporting(false);
    }
  }

  return (
    <div>
      <Button disabled={exporting} onClick={() => void exportItems}>
        {exporting ? "Exporting..." : "Export JSONL"}
      </Button>
      {exporting && <Button onClick={() => active.current?.abort()}>Cancel export</Button>}
      {error !== null && (
        <p className="mt-2 text-sm text-danger" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
