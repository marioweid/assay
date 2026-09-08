import type { ReactNode } from "react";
import { Link } from "react-router";

import { useAuth } from "@/auth/auth-context";
import { useApplicationCatalog } from "@/features/applications/application-catalog";

export function ApplicationsPage(): ReactNode {
  const { disconnect } = useAuth();
  const { applications, loading } = useApplicationCatalog();
  return (
    <main className="min-h-screen bg-canvas px-5 py-6 text-ink sm:px-8">
      <header className="flex items-center justify-between border-b border-line pb-4">
        <div>
          <p className="font-mono text-xs uppercase tracking-[0.16em] text-accent">Workspace</p>
          <h1 className="mt-1 text-xl font-semibold">Applications</h1>
        </div>
        <button className="text-sm font-medium text-muted hover:text-ink" onClick={disconnect}>
          Disconnect
        </button>
      </header>
      {loading ? (
        <p className="mt-12 text-sm text-muted">Loading applications...</p>
      ) : applications.length === 0 ? (
        <section className="mt-12 border-y border-line py-10">
          <h2 className="font-medium">No applications yet</h2>
          <p className="mt-2 text-sm text-muted">Create an application with the CLI or API.</p>
        </section>
      ) : (
        <div className="mt-6 overflow-x-auto border border-line bg-surface">
          <table className="w-full min-w-3xl border-collapse text-left text-sm">
            <thead className="border-b border-line bg-slate-100 text-xs uppercase text-muted">
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
