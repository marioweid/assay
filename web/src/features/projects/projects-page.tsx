import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { Link, useNavigate } from "react-router";

import { listProjects } from "@/api/generated/sdk.gen";
import type { ProjectResponse } from "@/api/generated/types.gen";
import { useAuth } from "@/auth/auth-context";
import { Button } from "@/components/ui/button";
import { EmptyState } from "@/components/empty-state";
import { LoadingState } from "@/components/loading-state";
import { PageHeading } from "@/components/page-heading";
import { ProblemState } from "@/components/problem-state";
import { ProjectForm } from "@/features/projects/project-form";

export function ProjectsPage(): ReactNode {
  const { disconnect } = useAuth();
  const navigate = useNavigate();
  const [projects, setProjects] = useState<ProjectResponse[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [refresh, setRefresh] = useState(0);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    setError(null);
    void listProjects({ signal: controller.signal, throwOnError: true })
      .then((response) => {
        if (!controller.signal.aborted) setProjects(response.data.items ?? []);
      })
      .catch(() => {
        if (!controller.signal.aborted) setError("Unable to load projects");
      });
    return () => controller.abort();
  }, [refresh]);

  return (
    <main className="min-h-screen bg-canvas px-5 py-6 text-ink sm:px-8">
      <header className="flex items-center justify-between border-b border-line pb-4">
        <nav aria-label="Workspace" className="flex items-center gap-2">
          <img alt="" className="h-6 w-6" src="/assay-icon.png" />
          <Link className="text-sm font-semibold" to="/apps">
            Assay
          </Link>
          <span aria-hidden="true" className="text-muted">
            /
          </span>
          <Link className="text-sm font-medium hover:text-ink" to="/apps">
            Applications
          </Link>
          <span aria-hidden="true" className="text-muted">
            /
          </span>
          <span aria-current="page" className="text-sm text-muted">
            Projects
          </span>
        </nav>
        <button className="text-sm font-medium text-muted hover:text-ink" onClick={disconnect}>
          Disconnect
        </button>
      </header>
      <div className="mt-6 flex items-center justify-between gap-4">
        <PageHeading
          description="Projects group applications and the ingest keys used to collect traces."
          title="Projects"
        />
        <Button onClick={() => setCreating(true)} variant="primary">
          New project
        </Button>
      </div>
      {projects === null && error === null ? (
        <LoadingState label="Loading projects" />
      ) : error !== null ? (
        <ProblemState
          detail={error}
          onRetry={() => setRefresh((value) => value + 1)}
          title="Projects unavailable"
        />
      ) : (projects ?? []).length === 0 ? (
        <EmptyState
          action={
            <Button onClick={() => setCreating(true)} variant="primary">
              Create project
            </Button>
          }
          description="Create a project to hold applications and API keys."
          title="No projects yet"
        />
      ) : (
        <div className="overflow-x-auto border border-line bg-surface">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-line bg-canvas text-xs uppercase text-muted">
              <tr>
                <th className="px-4 py-3">Name</th>
                <th className="px-4 py-3">Judge</th>
                <th className="px-4 py-3">Updated</th>
              </tr>
            </thead>
            <tbody>
              {(projects ?? []).map((project) => (
                <tr key={project.id} className="border-b border-line last:border-0">
                  <td className="px-4 py-3 font-medium">
                    <Link
                      className="text-accent-strong hover:underline"
                      to={`/projects/${project.id}`}
                    >
                      {project.name}
                    </Link>
                  </td>
                  <td className="px-4 py-3">
                    {project.judge_config?.has_api_key ? "Configured" : "Not configured"}
                  </td>
                  <td className="px-4 py-3 font-mono text-xs">
                    {new Date(project.updated_at).toLocaleDateString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {creating ? (
        <ProjectForm
          onClosed={() => setCreating(false)}
          onSaved={(projectID) => {
            setCreating(false);
            navigate(`/projects/${projectID}`);
          }}
        />
      ) : null}
    </main>
  );
}
