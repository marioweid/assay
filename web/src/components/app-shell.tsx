import {
  Activity,
  Database,
  FlaskConical,
  LineChart,
  MessagesSquare,
  Settings,
} from "lucide-react";
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
  { key: "sessions", label: "Sessions", icon: MessagesSquare },
  { key: "traces", label: "Traces", icon: Activity },
  { key: "datasets", label: "Datasets", icon: Database },
  { key: "runs", label: "Evaluations", icon: FlaskConical },
  { key: "metrics", label: "Score trends", icon: LineChart },
  { key: "settings", label: "Settings", icon: Settings },
] as const;

function sectionLabel(pathname: string): string {
  const sectionKey = pathname.split("/").filter(Boolean)[2] ?? "";
  return sections.find((section) => section.key === sectionKey)?.label ?? "Workspace";
}

export function AppShell(): ReactNode {
  const { appId = "" } = useParams();
  const { applications, error, loading, refresh } = useApplicationCatalog();
  const navigate = useNavigate();
  const location = useLocation();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const application = applications.find((item) => item.id === appId);

  if (application === undefined) {
    return (
      <main className="grid min-h-screen place-items-center bg-canvas px-6 text-ink">
        {loading ? (
          <LoadingState label="Loading application" />
        ) : error !== null ? (
          <ProblemState
            detail={error}
            onRetry={() => void refresh()}
            title="Applications unavailable"
          />
        ) : (
          <ProblemState
            detail="This application is missing or no longer available."
            title="Application not found"
          />
        )}
        {!loading && error === null && (
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
    <div className="min-h-screen bg-canvas text-ink md:grid md:grid-cols-[208px_minmax(0,1fr)]">
      <a className="skip-link" href="#workspace-content">
        Skip to content
      </a>
      <aside className="workspace-rail hidden min-h-screen border-r border-line bg-rail md:block">
        <Link className="workspace-logo" to="/apps">
          <span aria-hidden="true" className="workspace-mark">
            a
          </span>
          <span>
            assay<span className="text-accent">.</span>
          </span>
        </Link>
        <nav aria-label="Workspace" className="mt-10">
          <GlobalLink label="Applications" to="/apps" />
          <GlobalLink label="Projects" to="/projects" />
        </nav>
        <p
          title={application.name}
          className="mt-10 truncate border-t border-line px-2 pt-5 text-xs font-medium uppercase tracking-wider text-muted"
        >
          {application.name}
        </p>
        <SectionLinks application={application} onNavigate={null} />
      </aside>
      <div className="flex min-w-0 flex-col">
        <WorkspaceHeader
          navigationOpen={drawerOpen}
          onOpenNavigation={() => setDrawerOpen(true)}
          application={application}
          applications={applications}
          onSelectApplication={selectApplication}
          section={sectionLabel(location.pathname)}
        />
        <main className="workspace-content min-w-0" id="workspace-content" tabIndex={-1}>
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
      end
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
