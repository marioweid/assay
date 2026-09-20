import { useState } from "react";

import { Problem } from "@/api/errors";
import { deleteEvalRun } from "@/api/generated/sdk.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { fieldControlClass } from "@/components/ui/field";

export function DeleteRunAction({
  name,
  onConflict,
  onDeleted,
  runID,
}: {
  name: string;
  onConflict: (message: string) => void;
  onDeleted: () => void;
  runID: string;
}) {
  const [open, setOpen] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function remove(): Promise<void> {
    setDeleting(true);
    setError(null);
    try {
      await deleteEvalRun({ path: { id: runID }, throwOnError: true });
      onDeleted();
    } catch (reason) {
      if (reason instanceof Problem && reason.status === 409) {
        const message = "Run state changed. Only terminal runs can be deleted.";
        setError(message);
        onConflict(message);
      } else if (!(reason instanceof Problem) || reason.status === 0) {
        setUncertain(true);
        setError(
          "Deletion outcome is unknown. Return to the run list and refresh before trying again.",
        );
      } else {
        setError(reason.detail ?? reason.title);
      }
      setDeleting(false);
    }
  }

  return (
    <>
      <Button onClick={() => setOpen(true)} variant="danger">
        Delete run
      </Button>
      {open && (
        <Dialog
          onOpenChange={(next) => !next && setOpen(false)}
          open
          title="Delete evaluation run?"
        >
          <p className="mt-3 text-sm text-muted">
            This permanently deletes this run, its case outcomes, and all scores. This cannot be
            undone.
          </p>
          <label className="mt-4 block text-sm">
            <span className="font-medium text-ink">
              Type <span className="font-mono">{name}</span> to confirm
            </span>
            <input
              autoFocus
              className={fieldControlClass + " mt-1"}
              onChange={(event) => setConfirmation(event.target.value)}
              value={confirmation}
            />
          </label>
          {error !== null && (
            <p className="mt-3 text-sm text-danger" role="alert">
              {error}
            </p>
          )}
          <div className="mt-6 flex justify-end gap-3">
            <Button disabled={deleting} onClick={() => setOpen(false)}>
              Cancel
            </Button>
            <Button
              disabled={deleting || uncertain || confirmation !== name}
              onClick={() => void remove()}
              variant="danger"
            >
              {deleting ? "Deleting..." : "Delete run"}
            </Button>
          </div>
        </Dialog>
      )}
    </>
  );
}
