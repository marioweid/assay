import { Activity, Database, FlaskConical, LineChart, Settings } from "lucide-react";
import { useState } from "react";
import type { ReactNode } from "react";
import { Link, NavLink, Outlet, useLocation, useNavigate, useParams } from "react-router";

import type { ApplicationResponse } from "@/api/generated/types.gen";
import { LoadingState } from "@/components/loading-state";
import { ProblemState } from "@/components/problem-state";
import { Dialog } from "@/components/ui/dialog";
import { WorkspaceHeader } from "@/components/workspace-header";
import { useApplicationCatalog } from "@/features/applications/application-catalog";

const sections = [
  { key: "traces", label: "Traces", icon: Activity },
  { key: "datasets", label: "Datasets", icon: Database },
  { key: "runs", label: "Evaluations", icon: FlaskConical },
  { key: "metrics", label: "Score trends", icon: LineChart },
  { key: "settings", label: "Settings", icon: Settings },
] as const;

function sectionLabel(pathname: string): string {
  const last = pathname.split("/").filter(Boolean).at(-1) ?? "";
  return sections.find((section) => section.key === last)?.label ?? "Workspace";
}

export function AppShell(): ReactNode {
  const { appId = "" } = useParams();
  const { applications, loading } = useApplicationCatalog();
  const navigate = useNavigate();
  const location = useLocation();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const application = applications.find((item) => item.id === appId);

  if (application === undefined) {
    return (
      <main className="grid min-h-screen place-items-center bg-canvas px-6 text-ink">
        {loading ? (
          <LoadingState label="Loading application" />
        ) : (
          <ProblemState
            detail="This application is missing or no longer available."
            title="Application not found"
          />
        )}
        {!loading && (
          <Link className="mt-4 text-sm font-medium text-accent hover:underline" to="/apps">
            Back to applications
          </Link>
        )}
      </main>
    );
  }

  const selectApplication = (applicationID: string): void => {
    navigate(`/apps/${applicationID}/traces`);
  };

  return (
    <div className="min-h-screen bg-canvas text-ink md:grid md:grid-cols-[224px_minmax(0,1fr)]">
      <aside className="hidden min-h-screen border-r border-line bg-surface px-3 py-4 md:block">
        <Link
          className="flex items-center gap-2 px-2 text-sm font-semibold tracking-wide text-ink"
          to="/apps"
        >
          <img alt="" className="h-6 w-6" src="/assay-icon.png" />
          Assay
        </Link>
        <nav aria-label="Workspace" className="mt-6">
          <GlobalLink label="Applications" to="/apps" />
          <GlobalLink label="Projects" to="/projects" />
        </nav>
        <p className="mt-6 border-t border-line px-2 pt-4 text-xs font-medium uppercase tracking-wider text-muted">
          {application.name}
        </p>
        <SectionLinks application={application} onNavigate={null} />
      </aside>
      <div className="flex min-w-0 flex-col">
        <div className="flex items-center gap-2 border-b border-line bg-surface md:hidden">
          <button
            aria-controls="mobile-navigation"
            aria-expanded={drawerOpen}
            className="px-3 py-3 text-sm text-ink"
            onClick={() => setDrawerOpen(true)}
            type="button"
          >
            Open navigation
          </button>
        </div>
        <WorkspaceHeader
          application={application}
          applications={applications}
          onSelectApplication={selectApplication}
          section={sectionLabel(location.pathname)}
        />
        <main className="min-w-0 px-5 py-6 sm:px-8">
          <Outlet />
        </main>
      </div>
      <Dialog onOpenChange={setDrawerOpen} open={drawerOpen} title="Application navigation">
        <div data-testid="mobile-navigation" id="mobile-navigation">
          <SectionLinks application={application} onNavigate={() => setDrawerOpen(false)} />
          <Link
            className="mt-4 block px-4 py-2 text-sm text-muted"
            onClick={() => setDrawerOpen(false)}
            to="/apps"
          >
            All applications
          </Link>
        </div>
      </Dialog>
    </div>
  );
}

function GlobalLink({ label, to }: { label: string; to: string }): ReactNode {
  return (
    <NavLink
      className={({ isActive }) =>
        `block rounded-md px-3 py-2 text-sm ${
          isActive ? "bg-accent/10 font-medium text-accent" : "text-muted hover:text-ink"
        }`
      }
      to={to}
    >
      {label}
    </NavLink>
  );
}

function SectionLinks({
  application,
  onNavigate,
}: {
  application: ApplicationResponse;
  onNavigate: (() => void) | null;
}): ReactNode {
  return (
    <nav aria-label="Application" className="mt-2">
      {sections.map((section) => (
        <NavLink
          className={({ isActive }) =>
            `flex items-center gap-2 rounded-md px-3 py-2 text-sm ${
              isActive
                ? "bg-accent/10 font-medium text-accent"
                : "text-muted hover:bg-canvas hover:text-ink"
            }`
          }
          end={false}
          key={section.key}
          onClick={() => onNavigate?.()}
          to={`/apps/${application.id}/${section.key}`}
        >
          <section.icon aria-hidden="true" size={16} />
          {section.label}
        </NavLink>
      ))}
    </nav>
  );
}
