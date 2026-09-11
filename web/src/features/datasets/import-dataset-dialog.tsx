import { useState } from "react";

import { Problem } from "@/api/errors";
import { createDatasetItems } from "@/api/generated/sdk.gen";
import type { DatasetItemInput, DatasetItemResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { parseDatasetJsonl } from "@/features/datasets/dataset-jsonl";

const maxBytes = 5 * 1024 * 1024;
const maxItems = 1_000;

export function ImportDatasetDialog({
  datasetID,
  onCreated,
}: {
  datasetID: string;
  onCreated: (items: DatasetItemResponse[]) => void;
}): React.ReactElement {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<DatasetItemInput[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [importing, setImporting] = useState(false);

  async function select(file: File | undefined): Promise<void> {
    setItems(null);
    setError(null);
    if (file === undefined) return;
    if (file.size > maxBytes) {
      setError("JSONL files must be 5 MiB or smaller. Use the CLI for larger imports.");
      return;
    }
    try {
      const parsed = parseDatasetJsonl(await file.text());
      if (parsed.length > maxItems) {
        setError("JSONL imports are limited to 1,000 cases. Use the CLI for larger imports.");
        return;
      }
      setItems(parsed);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Unable to parse JSONL file");
    }
  }

  async function submit(): Promise<void> {
    if (items === null || items.length === 0) return;
    setImporting(true);
    setError(null);
    try {
      const response = await createDatasetItems({
        body: { items },
        path: { id: datasetID },
        throwOnError: true,
      });
      onCreated(response.data.items ?? []);
      setOpen(false);
    } catch (reason) {
      setError(
        reason instanceof Problem && reason.status === 409
          ? "No cases imported"
          : "Unable to import cases",
      );
    } finally {
      setImporting(false);
    }
  }

  return (
    <>
      <Button onClick={() => setOpen(true)}>Import JSONL</Button>
      {open && (
        <Dialog onOpenChange={(next) => !next && setOpen(false)} open title="Import dataset JSONL">
          <div className="mt-4 space-y-4">
            <p className="text-sm text-muted">
              Imports up to 1,000 cases from a JSONL file (5 MiB maximum). For larger imports, use
              the CLI.
            </p>
            <input
              accept=".jsonl,application/jsonl"
              aria-label="JSONL file"
              onChange={(event) => void select(event.target.files?.[0])}
              type="file"
            />
            {items !== null && (
              <p className="text-sm">
                {items.length} case{items.length === 1 ? "" : "s"} ready to import.
              </p>
            )}
            {error !== null && (
              <p className="text-sm text-danger" role="alert">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-3">
              <Button disabled={importing} onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button
                disabled={importing || items === null || items.length === 0}
                onClick={() => void submit()}
                variant="primary"
              >
                {importing ? "Importing..." : "Import"}
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </>
  );
}
