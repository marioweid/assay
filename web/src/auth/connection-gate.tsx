import { useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { Outlet } from "react-router";

import { useAuth } from "@/auth/auth-context";

export function ConnectionGate(): ReactNode {
  const auth = useAuth();
  const [candidate, setCandidate] = useState("");

  if (auth.status === "checking") {
    return (
      <main className="grid min-h-screen place-items-center bg-canvas px-6 text-ink">
        <p role="status" className="text-sm text-muted">
          Connecting to Assay...
        </p>
      </main>
    );
  }
  if (auth.status === "connected") {
    return <Outlet />;
  }

  function submit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    void auth.connect(candidate);
  }

  return (
    <main className="grid min-h-screen place-items-center bg-canvas px-6 py-12 text-ink">
      <section className="w-full max-w-md border border-line bg-surface p-7 shadow-sm">
        <p className="font-mono text-xs font-medium uppercase tracking-[0.18em] text-accent">
          Local evaluation workspace
        </p>
        <h1 className="mt-3 text-2xl font-semibold tracking-tight">Assay</h1>
        <p className="mt-2 text-sm leading-6 text-muted">
          Connect with the admin token for this single-user Assay instance.
        </p>
        <form className="mt-7 space-y-4" onSubmit={submit}>
          <label className="block text-sm font-medium" htmlFor="admin-token">
            Admin token
          </label>
          <input
            id="admin-token"
            type="password"
            autoComplete="current-password"
            value={candidate}
            onChange={(event) => setCandidate(event.target.value)}
            className="w-full border border-line bg-white px-3 py-2 text-sm"
            required
          />
          {auth.error ? (
            <p role="alert" className="text-sm text-danger">
              {auth.error}
            </p>
          ) : null}
          <button
            type="submit"
            className="w-full bg-accent-strong px-4 py-2 text-sm font-medium text-white"
          >
            Connect
          </button>
        </form>
      </section>
    </main>
  );
}
