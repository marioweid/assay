import { useState } from "react";

import { Problem } from "@/api/errors";
import { updateDataset } from "@/api/generated/sdk.gen";
import type { DatasetResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { fieldControlClass } from "@/components/ui/field";

export function DatasetMetadataDialog({
  dataset,
  onSaved,
}: {
  dataset: DatasetResponse;
  onSaved: (dataset: DatasetResponse) => void;
}): React.ReactElement {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState(dataset.name);
  const [description, setDescription] = useState(dataset.description ?? "");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function save(event: React.FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (name.trim() === "") return;
    setSaving(true);
    setError(null);
    try {
      const response = await updateDataset({
        body:
          description.trim() === ""
            ? { clear_description: true, name: name.trim() }
            : { description: description.trim(), name: name.trim() },
        path: { id: dataset.id },
        throwOnError: true,
      });
      onSaved(response.data);
      setOpen(false);
    } catch (reason) {
      setError(
        reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to save dataset",
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <>
      <Button aria-label="Edit dataset" onClick={() => setOpen(true)}>
        Edit dataset
      </Button>
      {open && (
        <Dialog onOpenChange={(next) => !next && setOpen(false)} open title="Edit dataset">
          <form className="mt-4 space-y-4" onSubmit={(event) => void save(event)}>
            <label className="block text-sm">
              <span className="font-medium">Dataset name</span>
              <input
                aria-label="Dataset name"
                className={`${fieldControlClass} mt-1`}
                disabled={saving}
                onChange={(event) => setName(event.target.value)}
                value={name}
              />
            </label>
            <label className="block text-sm">
              <span className="font-medium">Description (optional)</span>
              <textarea
                aria-label="Description (optional)"
                className={`${fieldControlClass} mt-1`}
                disabled={saving}
                onChange={(event) => setDescription(event.target.value)}
                value={description}
              />
            </label>
            {error !== null && (
              <p className="text-sm text-danger" role="alert">
                {error}
              </p>
            )}
            <div className="flex justify-end gap-3">
              <Button disabled={saving} onClick={() => setOpen(false)}>
                Cancel
              </Button>
              <Button disabled={saving || name.trim() === ""} type="submit" variant="primary">
                Save dataset
              </Button>
            </div>
          </form>
        </Dialog>
      )}
    </>
  );
}
