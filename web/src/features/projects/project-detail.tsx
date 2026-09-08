import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router";

import { Problem } from "@/api/errors";
import { deleteProject, getProject, listApplications } from "@/api/generated/sdk.gen";
import type { ApplicationResponse, ProjectResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/empty-state";
import { LoadingState } from "@/components/loading-state";
import { ProblemState } from "@/components/problem-state";
import { fieldControlClass } from "@/components/ui/field";
import { useApplicationCatalog } from "@/features/applications/application-catalog";
import { ApiKeysPanel } from "@/features/projects/api-keys-panel";
import { ProjectForm } from "@/features/projects/project-form";

export function ProjectDetail(): ReactNode {
  const { projectId = "" } = useParams();
  const navigate = useNavigate();
  const { refresh: refreshCatalog } = useApplicationCatalog();
  const [project, setProject] = useState<ProjectResponse | null>(null);
  const [applications, setApplications] = useState<ApplicationResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [renaming, setRenaming] = useState(false);
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [refresh, setRefresh] = useState(0);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    void getProject({ path: { id: projectId }, signal: controller.signal, throwOnError: true })
      .then((response) => {
        if (!controller.signal.aborted) setProject(response.data);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) {
          setError(
            reason instanceof Problem && reason.status === 404
              ? "This project was not found."
              : "Unable to load this project.",
          );
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [projectId, refresh]);

  useEffect(() => {
    const controller = new AbortController();
    void listApplications({
      query: { project_id: projectId },
      signal: controller.signal,
      throwOnError: true,
    })
      .then((response) => {
        if (!controller.signal.aborted) setApplications(response.data.items ?? []);
      })
      .catch(() => undefined);
    return () => controller.abort();
  }, [projectId]);

  if (loading) {
    return (
      <main className="min-h-screen bg-canvas px-5 py-6 text-ink sm:px-8">
        <LoadingState label="Loading project" />
      </main>
    );
  }
  if (project === null) {
    return (
      <main className="min-h-screen bg-canvas px-5 py-6 text-ink sm:px-8">
        {error !== null ? (
          <ProblemState detail={error} title="Project unavailable" />
        ) : (
          <ProblemState title="Project unavailable" />
        )}
        <Link className="mt-4 block text-sm font-medium text-accent hover:underline" to="/projects">
          Back to projects
        </Link>
      </main>
    );
  }

  return (
    <main className="min-h-screen bg-canvas px-5 py-6 text-ink sm:px-8">
      <nav aria-label="Breadcrumb" className="text-sm text-muted">
        <Link className="hover:text-ink" to="/projects">
          Projects
        </Link>
        <span aria-hidden="true" className="mx-1">
          /
        </span>
        <span aria-current="page">{project.name}</span>
      </nav>
      <header className="mt-2 flex items-center justify-between gap-4 border-b border-line pb-4">
        <div>
          <h1 className="text-2xl font-semibold">{project.name}</h1>
          <p className="mt-1 font-mono text-xs text-muted">{project.id}</p>
        </div>
        <div className="flex gap-2">
          <Button onClick={() => setRenaming(true)}>Rename</Button>
          <Button onClick={() => setConfirmingDelete(true)} variant="danger">
            Delete project
          </Button>
        </div>
      </header>

      <section className="mt-6 space-y-4">
        <h2 className="text-lg font-semibold">Applications</h2>
        {applications.length === 0 ? (
          <EmptyState
            description="Create an application from the Applications page to start collecting traces."
            title="No applications in this project"
          />
        ) : (
          <div className="overflow-x-auto border border-line bg-surface">
            <table className="w-full text-left text-sm">
              <thead className="border-b border-line bg-canvas text-xs uppercase text-muted">
                <tr>
                  <th className="px-4 py-3">Name</th>
                  <th className="px-4 py-3">Slug</th>
                </tr>
              </thead>
              <tbody>
                {applications.map((application) => (
                  <tr key={application.id} className="border-b border-line last:border-0">
                    <td className="px-4 py-3 font-medium">
                      <Link
                        className="text-accent-strong hover:underline"
                        to={`/apps/${application.id}/traces`}
                      >
                        {application.name}
                      </Link>
                    </td>
                    <td className="px-4 py-3 font-mono text-xs">{application.slug}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <div className="mt-8">
        <ApiKeysPanel projectID={project.id} />
      </div>

      {renaming ? (
        <ProjectForm
          initialName={project.name}
          onClosed={() => setRenaming(false)}
          onSaved={() => {
            setRenaming(false);
            setRefresh((value) => value + 1);
          }}
          projectID={project.id}
        />
      ) : null}
      {confirmingDelete ? (
        <DeleteProjectDialog
          onClose={() => setConfirmingDelete(false)}
          onDeleted={() => {
            void refreshCatalog();
            navigate("/projects");
          }}
          project={project}
        />
      ) : null}
    </main>
  );
}

function DeleteProjectDialog({
  project,
  onClose,
  onDeleted,
}: {
  project: ProjectResponse;
  onClose: () => void;
  onDeleted: () => void;
}): React.ReactElement {
  const [confirmation, setConfirmation] = useState("");
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const matches = confirmation.trim() === project.name;

  async function remove(): Promise<void> {
    setDeleting(true);
    setError(null);
    try {
      await deleteProject({ path: { id: project.id }, throwOnError: true });
      onDeleted();
    } catch (reason) {
      setError(
        reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to delete project",
      );
      setDeleting(false);
    }
  }

  return (
    <Dialog
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      open
      title="Delete project?"
    >
      <p className="mt-3 text-sm text-muted">
        This deletes the project, its applications, traces, datasets, evaluation runs, and scores.
        This cannot be undone.
      </p>
      <label className="mt-4 block text-sm">
        <span className="font-medium text-ink">
          Type <span className="font-mono">{project.name}</span> to confirm
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
        <Button disabled={deleting} onClick={onClose}>
          Cancel
        </Button>
        <Button disabled={deleting || !matches} onClick={() => void remove()} variant="danger">
          {deleting ? "Deleting..." : "Delete project"}
        </Button>
      </div>
    </Dialog>
  );
}
