import type { ReactNode } from "react";
import { Link } from "react-router";

import { useAuth } from "@/auth/auth-context";
import { PageHeading } from "@/components/page-heading";

export function ProjectsPage(): ReactNode {
  const { disconnect } = useAuth();
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
          <Link className="text-sm text-muted hover:text-ink" to="/apps">
            Applications
          </Link>
        </nav>
        <button className="text-sm font-medium text-muted hover:text-ink" onClick={disconnect}>
          Disconnect
        </button>
      </header>
      <PageHeading
        description="Group applications and ingest keys under one project."
        title="Projects"
      />
    </main>
  );
}
