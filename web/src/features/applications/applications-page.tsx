import { useState } from "react";
import type { ReactNode } from "react";
import { Link } from "react-router";

import { Problem } from "@/api/errors";
import { deleteApplication } from "@/api/generated/sdk.gen";
import type { ApplicationResponse } from "@/api/generated/types.gen";
import { useAuth } from "@/auth/auth-context";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { fieldControlClass } from "@/components/ui/field";
import { EmptyState } from "@/components/empty-state";
import { LoadingState } from "@/components/loading-state";
import { PageHeading } from "@/components/page-heading";
import { useApplicationCatalog } from "@/features/applications/application-catalog";
import { ApplicationForm } from "@/features/applications/application-form";

export function ApplicationsPage(): ReactNode {
  const { disconnect } = useAuth();
  const { applications, loading, refresh } = useApplicationCatalog();
  const [editing, setEditing] = useState<ApplicationResponse | "new" | null>(null);
  const [deleting, setDeleting] = useState<ApplicationResponse | null>(null);
  const close = (): void => setEditing(null);
  const saved = (): void => {
    close();
    void refresh();
  };
  return (
    <main className="min-h-screen bg-canvas px-5 py-6 text-ink sm:px-8">
      <header className="flex items-center justify-between border-b border-line pb-4">
        <div className="flex items-center gap-2">
          <img alt="" className="h-6 w-6" src="/assay-icon.png" />
          <Link className="text-sm font-semibold" to="/apps">
            Assay
          </Link>
          <span aria-hidden="true" className="text-muted">
            /
          </span>
          <Link className="text-sm text-muted hover:text-ink" to="/projects">
            Projects
          </Link>
        </div>
        <button className="text-sm font-medium text-muted hover:text-ink" onClick={disconnect}>
          Disconnect
        </button>
      </header>
      <div className="mt-6 flex items-start justify-between gap-4">
        <PageHeading
          description="Select an application to review traces, datasets, and evaluation runs."
          title="Applications"
        />
        <Button onClick={() => setEditing("new")} variant="primary">
          New application
        </Button>
      </div>
      {loading ? (
        <LoadingState label="Loading applications" />
      ) : applications.length === 0 ? (
        <EmptyState
          action={
            <Button onClick={() => setEditing("new")} variant="primary">
              Create application
            </Button>
          }
          description="Create an application here, or use the CLI or API to start collecting traces."
          title="No applications yet"
        />
      ) : (
        <ApplicationTable applications={applications} onDelete={setDeleting} onEdit={setEditing} />
      )}
      {editing !== null && (
        <ApplicationForm
          {...(editing === "new" ? {} : { application: editing })}
          onClosed={close}
          onSaved={saved}
        />
      )}
      {deleting !== null && (
        <DeleteApplicationDialog
          application={deleting}
          onClosed={() => setDeleting(null)}
          onDeleted={() => {
            setDeleting(null);
            void refresh();
          }}
        />
      )}
    </main>
  );
}

function ApplicationTable({
  applications,
  onEdit,
  onDelete,
}: {
  applications: ApplicationResponse[];
  onEdit: (application: ApplicationResponse) => void;
  onDelete: (application: ApplicationResponse) => void;
}): React.ReactElement {
  return (
    <div className="overflow-x-auto border border-line bg-surface">
      <table className="w-full min-w-3xl border-collapse text-left text-sm">
        <thead className="border-b border-line bg-canvas text-xs uppercase text-muted">
          <tr>
            <th className="px-4 py-3">Name</th>
            <th className="px-4 py-3">Slug</th>
            <th className="px-4 py-3">Project</th>
            <th className="px-4 py-3">Automatic scorers</th>
            <th className="px-4 py-3">
              <span className="sr-only">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {applications.map((application) => (
            <tr className="border-b border-line last:border-0" key={application.id}>
              <td className="px-4 py-3 font-medium">
                <Link
                  className="text-accent-strong hover:underline"
                  to={`/apps/${application.id}/traces`}
                >
                  {application.name}
                </Link>
              </td>
              <td className="px-4 py-3 font-mono text-xs">{application.slug}</td>
              <td className="px-4 py-3 font-mono text-xs">{application.project_id}</td>
              <td className="px-4 py-3">{application.auto_score_scorers?.join(", ") || "None"}</td>
              <td className="space-x-1 px-4 py-3 text-right">
                <Button
                  aria-label={`Edit ${application.name}`}
                  onClick={() => onEdit(application)}
                  variant="ghost"
                >
                  Edit
                </Button>
                <Button
                  aria-label={`Delete ${application.name}`}
                  onClick={() => onDelete(application)}
                  variant="ghost"
                >
                  Delete
                </Button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function DeleteApplicationDialog({
  application,
  onClosed,
  onDeleted,
}: {
  application: ApplicationResponse;
  onClosed: () => void;
  onDeleted: () => void;
}): React.ReactElement {
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const matches = confirmation.trim() === application.name;

  async function remove(): Promise<void> {
    setSubmitting(true);
    setError(null);
    try {
      await deleteApplication({ path: { id: application.id }, throwOnError: true });
      onDeleted();
    } catch (reason) {
      setError(
        reason instanceof Problem
          ? (reason.detail ?? reason.title)
          : "Unable to delete application",
      );
      setSubmitting(false);
    }
  }

  return (
    <Dialog onOpenChange={(open) => !open && onClosed()} open title={`Delete ${application.name}?`}>
      <p className="mt-3 text-sm text-muted">
        This permanently deletes the application and its traces, datasets, evaluation runs, and
        scores.
      </p>
      <label className="mt-4 block text-sm">
        <span className="font-medium">Type {application.name} to confirm</span>
        <input
          autoFocus
          className={`${fieldControlClass} mt-1`}
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
        <Button disabled={submitting} onClick={onClosed}>
          Cancel
        </Button>
        <Button disabled={submitting || !matches} onClick={() => void remove()} variant="danger">
          Delete application
        </Button>
      </div>
    </Dialog>
  );
}
