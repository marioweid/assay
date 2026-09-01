import type { ReactNode } from "react";
import { Navigate, Route, Routes } from "react-router";

import { ConnectionGate } from "@/auth/connection-gate";
import { AppShell } from "@/components/app-shell";
import { DatasetDetail } from "@/features/datasets/dataset-detail";
import { DatasetsPage } from "@/features/datasets/datasets-page";
import { TraceDetail } from "@/features/traces/trace-detail";
import { TracesPage } from "@/features/traces/traces-page";
import { ApplicationsPage } from "@/features/applications/applications-page";

export function AppRoutes(): ReactNode {
  return (
    <Routes>
      <Route element={<ConnectionGate />}>
        <Route path="/" element={<Navigate replace to="/apps" />} />
        <Route path="/apps" element={<ApplicationsPage />} />
        <Route path="/apps/:appId" element={<AppShell />}>
          <Route index element={<Navigate replace to="traces" />} />
          <Route path="traces" element={<TracesPage />} />
          <Route path="traces/:traceId" element={<TraceDetail />} />
          <Route path="datasets" element={<DatasetsPage />} />
          <Route path="datasets/:datasetId" element={<DatasetDetail />} />
          <Route path="runs" element={<Workspace title="Runs" />} />
        </Route>
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  );
}

function NotFound(): ReactNode {
  return (
    <main className="px-5 py-6 sm:px-8">
      <h1 className="text-xl font-semibold">Page not found</h1>
    </main>
  );
}

function Workspace({ title }: { title: string }): ReactNode {
  return (
    <div>
      <h1 className="text-xl font-semibold">{title}</h1>
    </div>
  );
}
