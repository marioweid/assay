import { useEffect, useRef, useState } from "react";

import { Problem } from "@/api/errors";
import { createProject, updateProject } from "@/api/generated/sdk.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { fieldControlClass } from "@/components/ui/field";

type ProjectFormProps = {
  projectID?: string;
  initialName?: string;
  onClosed: () => void;
  onSaved: (projectID: string) => void;
};

export function ProjectForm({
  projectID,
  initialName = "",
  onClosed,
  onSaved,
}: ProjectFormProps): React.ReactElement {
  const activeRequest = useRef<AbortController | null>(null);
  const [name, setName] = useState(initialName);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => () => activeRequest.current?.abort(), []);

  async function submit(event: React.FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    const trimmed = name.trim();
    if (trimmed === "" || activeRequest.current !== null) return;
    const controller = new AbortController();
    activeRequest.current = controller;
    setSubmitting(true);
    setError(null);
    try {
      let savedID = projectID;
      if (projectID === undefined) {
        const created = await createProject({
          body: { name: trimmed },
          signal: controller.signal,
          throwOnError: true,
        });
        savedID = created.data.id;
      } else {
        await updateProject({
          path: { id: projectID },
          body: { name: trimmed },
          signal: controller.signal,
          throwOnError: true,
        });
      }
      if (savedID !== undefined && !controller.signal.aborted) onSaved(savedID);
    } catch (reason) {
      if (controller.signal.aborted) return;
      if (reason instanceof Problem) {
        if (reason.status === 401) return;
        setError(reason.detail ?? reason.title);
      } else {
        setError(projectID === undefined ? "Unable to create project" : "Unable to rename project");
      }
    } finally {
      activeRequest.current = null;
      if (!controller.signal.aborted) setSubmitting(false);
    }
  }

  return (
    <Dialog
      onOpenChange={(open) => {
        if (!open) onClosed();
      }}
      open
      title={projectID === undefined ? "New project" : "Rename project"}
    >
      <form className="mt-4 space-y-4" onSubmit={(event) => void submit(event)}>
        <label className="block text-sm">
          <span className="font-medium text-ink">Project name</span>
          <input
            autoFocus
            className={fieldControlClass + " mt-1"}
            onChange={(event) => setName(event.target.value)}
            value={name}
          />
        </label>
        {error !== null && (
          <p className="text-sm text-danger" role="alert">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-3">
          <Button onClick={onClosed}>Cancel</Button>
          <Button disabled={submitting || name.trim() === ""} type="submit" variant="primary">
            {submitting
              ? projectID === undefined
                ? "Creating..."
                : "Saving..."
              : projectID === undefined
                ? "Create project"
                : "Save name"}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
