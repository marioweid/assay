import { LogOut } from "lucide-react";

import { useAuth } from "@/auth/auth-context";

export function ConnectionControl() {
  const { localMode, disconnect } = useAuth();
  if (localMode) {
    return (
      <span
        className="inline-flex min-h-9 shrink-0 items-center rounded-lg border border-accent/40 px-2 text-xs text-accent"
        title="No admin token required. Anyone who can reach this server has management access."
      >
        Local mode
      </span>
    );
  }
  return (
    <button
      className="workspace-disconnect session-icon-button"
      title="Disconnect"
      onClick={disconnect}
      type="button"
    >
      <LogOut aria-hidden="true" size={16} />
      <span className="sr-only sm:not-sr-only">Disconnect</span>
    </button>
  );
}
