import { useState } from "react";

import { Problem } from "@/api/errors";
import { deleteDatasetItem } from "@/api/generated/sdk.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";

export function DatasetItemDelete({
  datasetID,
  itemID,
  label,
  onDeleted,
}: {
  datasetID: string;
  itemID: string;
  label: string;
  onDeleted: () => void;
}): React.ReactElement {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState(false);

  async function remove(): Promise<void> {
    setDeleting(true);
    setError(null);
    try {
      await deleteDatasetItem({ path: { id: datasetID, itemId: itemID }, throwOnError: true });
      onDeleted();
      setOpen(false);
    } catch (reason) {
      setError(
        reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to delete item",
      );
    } finally {
      setDeleting(false);
    }
  }

  return (
    <>
      <Button aria-label={`Delete ${label}`} onClick={() => setOpen(true)} variant="ghost">
        Delete
      </Button>
      {open && (
        <Dialog onOpenChange={(next) => !next && setOpen(false)} open title="Delete dataset case?">
          <div className="mt-4 space-y-4">
            <p className="text-sm text-muted">
              This removes the case. Historical evaluation evidence remains available.
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
                {deleting ? "Deleting..." : "Delete item"}
              </Button>
            </div>
          </div>
        </Dialog>
      )}
    </>
  );
}
