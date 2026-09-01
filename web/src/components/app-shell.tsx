import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { NavLink, Outlet, useNavigate, useParams } from "react-router";

import { useAuth } from "@/auth/auth-context";

const sections = ["traces", "datasets", "runs"] as const;

export function AppShell(): ReactNode {
  const { appId } = useParams();
  const { applications, disconnect } = useAuth();
  const navigate = useNavigate();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const firstDrawerLink = useRef<HTMLAnchorElement>(null);
  const application = applications.find((item) => item.id === appId);

  useEffect(() => {
    if (drawerOpen) {
      firstDrawerLink.current?.focus();
    }
  }, [drawerOpen]);

  if (application === undefined) {
    return <main className="p-8">Application not found.</main>;
  }

  const navigation = (mobile: boolean): ReactNode => (
    <nav aria-label="Application">
      {sections.map((section, index) => (
        <NavLink
          key={section}
          ref={mobile && index === 0 ? firstDrawerLink : undefined}
          to={`/apps/${application.id}/${section}`}
          onClick={() => setDrawerOpen(false)}
          className={({ isActive }) =>
            `block border-l-2 px-4 py-2 text-sm capitalize ${
              isActive
                ? "border-blue-300 bg-white/10 text-white"
                : "border-transparent text-slate-300"
            }`
          }
        >
          {section}
        </NavLink>
      ))}
    </nav>
  );

  return (
    <div className="min-h-screen bg-canvas text-ink md:grid md:grid-cols-[15rem_1fr]">
      <aside className="hidden min-h-screen bg-rail px-3 py-5 text-white md:block">
        <p className="px-4 font-mono text-sm font-semibold tracking-[0.16em]">ASSAY</p>
        <p className="mt-6 truncate px-4 text-sm font-medium">{application.name}</p>
        <div className="mt-4">{navigation(false)}</div>
      </aside>
      <div>
        <header className="flex items-center gap-3 border-b border-line bg-surface px-4 py-3">
          <button className="text-sm md:hidden" onClick={() => setDrawerOpen(true)}>
            Open navigation
          </button>
          <select
            aria-label="Application"
            value={application.id}
            onChange={(event) => navigate(`/apps/${event.target.value}/traces`)}
            className="max-w-64 border border-line bg-white px-2 py-1.5 text-sm"
          >
            {applications.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
              </option>
            ))}
          </select>
          <button className="ml-auto text-sm text-muted" onClick={disconnect}>
            Disconnect
          </button>
        </header>
        <Outlet />
      </div>
      {drawerOpen ? (
        <div
          role="dialog"
          aria-modal="true"
          aria-label="Application navigation"
          className="fixed inset-0 z-10 bg-black/30 md:hidden"
        >
          <div className="h-full w-72 bg-rail px-3 py-5 text-white">
            <button className="mb-5 px-4 text-sm" onClick={() => setDrawerOpen(false)}>
              Close navigation
            </button>
            {navigation(true)}
          </div>
        </div>
      ) : null}
    </div>
  );
}
