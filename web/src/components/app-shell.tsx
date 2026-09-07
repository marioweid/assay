import { useEffect, useRef, useState } from "react";
import type { KeyboardEvent, ReactNode, RefObject } from "react";
import { NavLink, Outlet, useNavigate, useParams } from "react-router";

import { useAuth } from "@/auth/auth-context";

const sections = ["traces", "datasets", "runs", "metrics"] as const;

export function AppShell(): ReactNode {
  const { appId } = useParams();
  const { applications, disconnect } = useAuth();
  const navigate = useNavigate();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const openButton = useRef<HTMLButtonElement>(null);
  const application = applications.find((item) => item.id === appId);

  if (application === undefined) {
    return <main className="p-8">Application not found.</main>;
  }

  const navigation = (): ReactNode => (
    <nav aria-label="Application">
      {sections.map((section) => (
        <NavLink
          key={section}
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
        <div className="mt-4">{navigation()}</div>
      </aside>
      <div>
        <header className="flex items-center gap-3 border-b border-line bg-surface px-4 py-3">
          <button
            aria-controls="mobile-navigation"
            aria-expanded={drawerOpen}
            className="text-sm md:hidden"
            onClick={() => setDrawerOpen(true)}
            ref={openButton}
          >
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
        <main className="px-5 py-6 sm:px-8">
          <Outlet />
        </main>
      </div>
      {drawerOpen ? (
        <MobileDrawer onClose={() => setDrawerOpen(false)} returnFocus={openButton}>
          {navigation()}
        </MobileDrawer>
      ) : null}
    </div>
  );
}

type MobileDrawerProps = {
  children: ReactNode;
  onClose: () => void;
  returnFocus: RefObject<HTMLButtonElement | null>;
};

function MobileDrawer({ children, onClose, returnFocus }: MobileDrawerProps) {
  const dialog = useRef<HTMLDivElement>(null);

  useEffect(() => {
    dialog.current?.querySelector<HTMLAnchorElement>("a")?.focus();
    return () => returnFocus.current?.focus();
  }, [returnFocus]);

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>): void {
    if (event.key === "Escape") {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key !== "Tab" || dialog.current === null) return;
    const focusable = Array.from(dialog.current.querySelectorAll<HTMLElement>("a, button"));
    const first = focusable[0];
    const last = focusable.at(-1);
    if (first === undefined || last === undefined) return;
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }

  return (
    <div
      aria-label="Application navigation"
      aria-modal="true"
      className="fixed inset-0 z-10 bg-black/30 md:hidden"
      id="mobile-navigation"
      onKeyDown={handleKeyDown}
      ref={dialog}
      role="dialog"
    >
      <div className="h-full w-72 bg-rail px-3 py-5 text-white">
        {children}
        <button className="mt-5 px-4 text-sm" onClick={onClose}>
          Close navigation
        </button>
      </div>
    </div>
  );
}
