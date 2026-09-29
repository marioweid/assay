import { Menu } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";

import type { ApplicationResponse } from "@/api/generated/types.gen";
import { ConnectionControl } from "@/auth/connection-control";
import { useTheme } from "@/app/theme-provider";

export type WorkspaceHeaderProps = {
  application: ApplicationResponse;
  applications: ApplicationResponse[];
  section: string;
  onSelectApplication: (applicationID: string) => void;
  onOpenNavigation: () => void;
  navigationOpen: boolean;
};

export function WorkspaceHeader({
  application,
  applications,
  section,
  onSelectApplication,
  onOpenNavigation,
  navigationOpen,
}: WorkspaceHeaderProps): ReactNode {
  const theme = useTheme();

  return (
    <header className="workspace-header">
      <button
        aria-label="Open navigation"
        aria-controls="mobile-navigation"
        aria-expanded={navigationOpen}
        className="workspace-menu session-icon-button md:hidden"
        onClick={onOpenNavigation}
        type="button"
      >
        <Menu aria-hidden="true" size={18} />
      </button>
      <nav aria-label="Breadcrumb" className="hidden min-w-0 items-center gap-2 text-xs lg:flex">
        <Link className="text-muted hover:text-ink" to="/apps">
          Applications
        </Link>
        <span aria-hidden="true" className="text-muted">
          /
        </span>
        <Link className="truncate font-medium text-ink" to={`/apps/${application.id}`}>
          {application.name}
        </Link>
        <span aria-hidden="true" className="text-muted">
          /
        </span>
        <span aria-current="page" className="font-semibold">
          {section}
        </span>
      </nav>
      <select
        aria-label="Application"
        className="workspace-application"
        onChange={(event) => onSelectApplication(event.target.value)}
        value={application.id}
      >
        {applications.map((item) => (
          <option key={item.id} value={item.id}>
            {item.name}
          </option>
        ))}
      </select>
      <select
        aria-label="Theme"
        className="workspace-theme"
        onChange={(event) => theme.setPreference(event.target.value as "system" | "light" | "dark")}
        value={theme.preference}
      >
        <option value="system">System theme</option>
        <option value="light">Light</option>
        <option value="dark">Dark</option>
      </select>
      <ConnectionControl />
    </header>
  );
}
