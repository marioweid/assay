import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { Problem } from "@/api/errors";
import { createDataset } from "@/api/generated/sdk.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { fieldControlClass } from "@/components/ui/field";

export function CreateDatasetDialog({ appID, onClose }: { appID: string; onClose: () => void }) {
  const navigate = useNavigate();
  const activeRequest = useRef<AbortController | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => () => activeRequest.current?.abort(), []);

  async function submit(event: React.FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (name.trim() === "" || activeRequest.current !== null) return;
    const controller = new AbortController();
    activeRequest.current = controller;
    setSubmitting(true);
    setError(null);
    try {
      const response = await createDataset({
        body: { application_id: appID, name: name.trim(), description: description.trim() },
        signal: controller.signal,
        throwOnError: true,
      });
      if (!controller.signal.aborted) navigate(`/apps/${appID}/datasets/${response.data.id}`);
    } catch (reason) {
      if (!controller.signal.aborted)
        setError(
          reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to create dataset",
        );
    } finally {
      activeRequest.current = null;
      if (!controller.signal.aborted) setSubmitting(false);
    }
  }

  return (
    <Dialog
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      open
      title="Create dataset"
    >
      <form className="mt-4 space-y-4" onSubmit={(event) => void submit(event)}>
        <p className="text-sm text-muted">
          Create a collection of evaluation cases for this application.
        </p>
        <label className="block text-sm">
          <span className="font-medium text-ink">Dataset name</span>
          <input
            autoFocus
            className={fieldControlClass + " mt-1"}
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            disabled={submitting}
          />
        </label>
        <label className="block text-sm">
          <span className="font-medium text-ink">Description (optional)</span>
          <textarea
            className={fieldControlClass + " mt-1"}
            rows={3}
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            disabled={submitting}
          />
        </label>
        {error !== null && (
          <p className="text-sm text-danger" role="alert">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-3">
          <Button onClick={onClose}>Cancel</Button>
          <Button disabled={submitting || name.trim() === ""} type="submit" variant="primary">
            {submitting ? "Creating..." : "Save dataset"}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
