import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";

import { Problem } from "@/api/errors";
import { createDataset } from "@/api/generated/sdk.gen";
import { Modal } from "@/components/modal";

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
    <Modal label="Create dataset" onClose={onClose}>
      <form
        className="w-full max-w-lg space-y-4 border border-line bg-white p-6 shadow-xl"
        onSubmit={(event) => void submit(event)}
      >
        <h2 className="text-xl font-semibold">Create dataset</h2>
        <p className="text-sm text-muted">
          Create a collection of evaluation cases for this application.
        </p>
        <label className="block text-sm">
          Dataset name
          <input
            className="mt-1 block w-full border border-line bg-white px-3 py-2"
            required
            value={name}
            onChange={(event) => setName(event.target.value)}
            disabled={submitting}
          />
        </label>
        <label className="block text-sm">
          Description (optional)
          <textarea
            className="mt-1 block w-full border border-line bg-white px-3 py-2"
            rows={3}
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            disabled={submitting}
          />
        </label>
        {error !== null && (
          <p className="text-sm text-red-700" role="alert">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-3">
          <button className="border border-line px-4 py-2 text-sm" type="button" onClick={onClose}>
            Cancel
          </button>
          <button
            className="bg-blue-700 px-4 py-2 text-sm text-white disabled:opacity-50"
            type="submit"
            disabled={submitting || name.trim() === ""}
          >
            {submitting ? "Creating..." : "Save dataset"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
