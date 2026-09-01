import type { ReactNode } from "react";
import { Navigate, Route, Routes } from "react-router";

import { ConnectionGate } from "@/auth/connection-gate";
import { AppShell } from "@/components/app-shell";
import { ApplicationsPage } from "@/features/applications/applications-page";

export function AppRoutes(): ReactNode {
  return (
    <Routes>
      <Route element={<ConnectionGate />}>
        <Route path="/" element={<Navigate replace to="/apps" />} />
        <Route path="/apps" element={<ApplicationsPage />} />
        <Route path="/apps/:appId" element={<AppShell />}>
          <Route index element={<Navigate replace to="traces" />} />
          <Route path="traces" element={<Workspace title="Traces" />} />
          <Route path="datasets" element={<Workspace title="Datasets" />} />
          <Route path="runs" element={<Workspace title="Runs" />} />
        </Route>
        <Route path="*" element={<Workspace title="Page not found" />} />
      </Route>
    </Routes>
  );
}

function Workspace({ title }: { title: string }): ReactNode {
  return (
    <main className="px-5 py-6 sm:px-8">
      <h1 className="text-xl font-semibold">{title}</h1>
    </main>
  );
}
