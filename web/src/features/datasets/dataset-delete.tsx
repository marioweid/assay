import { useState } from "react";

import { Problem } from "@/api/errors";
import { deleteDataset } from "@/api/generated/sdk.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";

export function DatasetDelete({
  datasetID,
  onDeleted,
}: {
  datasetID: string;
  onDeleted: () => void;
}): React.ReactElement {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  async function remove(): Promise<void> {
    setDeleting(true);
    setError(null);
    try {
      await deleteDataset({ path: { id: datasetID }, throwOnError: true });
      onDeleted();
    } catch (reason) {
      setError(
        reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to delete dataset",
      );
    } finally {
      setDeleting(false);
    }
  }

  return (
    <>
      <Button onClick={() => setOpen(true)} variant="danger">
        Delete dataset
      </Button>
      {open && (
        <Dialog onOpenChange={(next) => !next && setOpen(false)} open title="Delete dataset?">
          <div className="mt-4 space-y-4">
            <p className="text-sm text-muted">
              This removes this dataset, its cases, and associated evaluation runs and scores.
            </p>
            {error !== null && (
              <p className="text-sm text-danger" role="alert">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-3">
              <Button disabled={deleting} onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button disabled={deleting} onClick={() => void remove()} variant="danger">
                {deleting ? "Deleting..." : "Delete dataset"}
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </>
  );
}
