import { useEffect, useRef, useState } from "react";

import { Problem } from "@/api/errors";
import { createApplication, listProjects, updateApplication } from "@/api/generated/sdk.gen";
import type { ApplicationResponse, ProjectResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { fieldControlClass } from "@/components/ui/field";

export function ApplicationForm({
  application,
  onClosed,
  onSaved,
}: {
  application?: ApplicationResponse;
  onClosed: () => void;
  onSaved: () => void;
}): React.ReactElement {
  const activeRequest = useRef<AbortController | null>(null);
  const [projects, setProjects] = useState<ProjectResponse[]>([]);
  const [name, setName] = useState(application?.name ?? "");
  const [slug, setSlug] = useState(application?.slug ?? "");
  const [projectID, setProjectID] = useState(application?.project_id ?? "");
  const [config, setConfig] = useState(() => JSON.stringify(application?.config ?? {}, null, 2));
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    if (application !== undefined) return () => controller.abort();
    void listProjects({ signal: controller.signal, throwOnError: true })
      .then((response) => {
        if (!controller.signal.aborted) setProjects(response.data.items ?? []);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError("Unable to load projects");
      });
    return () => controller.abort();
  }, [application]);
  useEffect(() => () => activeRequest.current?.abort(), []);

  async function submit(event: React.FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    if (activeRequest.current !== null) return;
    const parsedConfig = parseConfig(config);
    if (parsedConfig === null) {
      setError("Configuration must be a JSON object");
      return;
    }
    if (name.trim() === "" || slug.trim() === "" || projectID === "") return;
    const controller = new AbortController();
    activeRequest.current = controller;
    setSubmitting(true);
    setError(null);
    try {
      if (application === undefined) {
        await createApplication({
          body: {
            config: parsedConfig,
            name: name.trim(),
            project_id: projectID,
            slug: slug.trim(),
          },
          signal: controller.signal,
          throwOnError: true,
        });
      } else {
        await updateApplication({
          body: { config: parsedConfig, name: name.trim(), slug: slug.trim() },
          path: { id: application.id },
          signal: controller.signal,
          throwOnError: true,
        });
      }
      if (!controller.signal.aborted) onSaved();
    } catch (reason) {
      if (!controller.signal.aborted) {
        setError(
          reason instanceof Problem
            ? (reason.detail ?? reason.title)
            : "Unable to save application",
        );
      }
    } finally {
      activeRequest.current = null;
      if (!controller.signal.aborted) setSubmitting(false);
    }
  }

  const editing = application !== undefined;
  return (
    <Dialog
      onOpenChange={(open) => !open && onClosed()}
      open
      title={editing ? "Edit application" : "New application"}
    >
      <form className="mt-4 space-y-4" onSubmit={(event) => void submit(event)}>
        <label className="block text-sm">
          <span className="font-medium text-ink">Application name</span>
          <input
            autoFocus
            className={`${fieldControlClass} mt-1`}
            onChange={(event) => setName(event.target.value)}
            value={name}
          />
        </label>
        <label className="block text-sm">
          <span className="font-medium text-ink">Slug</span>
          <input
            className={`${fieldControlClass} mt-1`}
            onChange={(event) => setSlug(event.target.value)}
            value={slug}
          />
          {editing && (
            <span className="mt-1 block text-xs text-muted">
              Changing this requires emitters to use the new slug.
            </span>
          )}
        </label>
        {editing ? (
          <p className="text-sm text-muted">Project ownership cannot be changed.</p>
        ) : (
          <label className="block text-sm">
            <span className="font-medium text-ink">Project</span>
            <select
              className={`${fieldControlClass} mt-1`}
              onChange={(event) => setProjectID(event.target.value)}
              value={projectID}
            >
              <option value="">Select a project</option>
              {projects.map((project) => (
                <option key={project.id} value={project.id}>
                  {project.name}
                </option>
              ))}
            </select>
          </label>
        )}
        <label className="block text-sm">
          <span className="font-medium text-ink">Advanced configuration JSON</span>
          <textarea
            className={`${fieldControlClass} mt-1 min-h-28 font-mono`}
            onChange={(event) => setConfig(event.target.value)}
            value={config}
          />
        </label>
        {error !== null && (
          <p className="text-sm text-danger" role="alert">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-3">
          <Button onClick={onClosed}>Cancel</Button>
          <Button
            disabled={submitting || name.trim() === "" || slug.trim() === "" || projectID === ""}
            type="submit"
            variant="primary"
          >
            {submitting ? "Saving..." : editing ? "Save application" : "Create application"}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}

function parseConfig(value: string): Record<string, unknown> | null {
  try {
    const parsed: unknown = JSON.parse(value);
    return parsed !== null && !Array.isArray(parsed) && typeof parsed === "object"
      ? (parsed as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}
