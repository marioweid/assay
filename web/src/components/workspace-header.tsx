import type { ReactNode } from "react";
import { Link } from "react-router";

import type { ApplicationResponse } from "@/api/generated/types.gen";
import { useAuth } from "@/auth/auth-context";
import { useTheme } from "@/app/theme-provider";

export type WorkspaceHeaderProps = {
  application: ApplicationResponse;
  applications: ApplicationResponse[];
  section: string;
  onSelectApplication: (applicationID: string) => void;
};

export function WorkspaceHeader({
  application,
  applications,
  section,
  onSelectApplication,
}: WorkspaceHeaderProps): ReactNode {
  const { disconnect } = useAuth();
  const theme = useTheme();

  return (
    <header className="flex h-14 items-center gap-3 border-b border-line bg-surface px-4">
      <nav aria-label="Breadcrumb" className="hidden min-w-0 items-center gap-1 text-sm sm:flex">
        <Link className="text-muted hover:text-ink" to="/apps">
          Applications
        </Link>
        <span aria-hidden="true" className="text-muted">
          /
        </span>
        <Link
          aria-current="page"
          className="truncate font-medium text-ink"
          to={`/apps/${application.id}`}
        >
          {application.name}
        </Link>
        <span aria-hidden="true" className="text-muted">
          /
        </span>
        <span className="text-muted">{section}</span>
      </nav>
      <select
        aria-label="Application"
        className="max-w-64 min-w-0 truncate rounded-md border border-line bg-surface px-2 py-1.5 text-sm text-ink"
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
        className="rounded-md border border-line bg-surface px-2 py-1.5 text-sm text-ink"
        onChange={(event) => theme.setPreference(event.target.value as "system" | "light" | "dark")}
        value={theme.preference}
      >
        <option value="system">System theme</option>
        <option value="light">Light</option>
        <option value="dark">Dark</option>
      </select>
      <button
        className="ml-auto text-sm text-muted hover:text-ink"
        onClick={disconnect}
        type="button"
      >
        Disconnect
      </button>
    </header>
  );
}
