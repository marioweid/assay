import type { ReactNode } from "react";
import { Link } from "react-router";

import { useAuth } from "@/auth/auth-context";
import { EmptyState } from "@/components/empty-state";
import { LoadingState } from "@/components/loading-state";
import { PageHeading } from "@/components/page-heading";
import { useApplicationCatalog } from "@/features/applications/application-catalog";

export function ApplicationsPage(): ReactNode {
  const { disconnect } = useAuth();
  const { applications, loading } = useApplicationCatalog();
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
      <PageHeading
        description="Select an application to review traces, datasets, and evaluation runs."
        title="Applications"
      />
      {loading ? (
        <LoadingState label="Loading applications" />
      ) : applications.length === 0 ? (
        <EmptyState
          description="Create an application with the CLI or API to start collecting traces."
          title="No applications yet"
        />
      ) : (
        <div className="overflow-x-auto border border-line bg-surface">
          <table className="w-full min-w-3xl border-collapse text-left text-sm">
            <thead className="border-b border-line bg-canvas text-xs uppercase text-muted">
              <tr>
                <th className="px-4 py-3">Name</th>
                <th className="px-4 py-3">Slug</th>
                <th className="px-4 py-3">Project</th>
                <th className="px-4 py-3">Automatic scorers</th>
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
                  <td className="px-4 py-3 font-mono text-xs">{application.project_id}</td>
                  <td className="px-4 py-3">
                    {application.auto_score_scorers?.join(", ") || "None"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}
