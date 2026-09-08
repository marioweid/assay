import type { ReactNode } from "react";
import { Navigate, Route, Routes } from "react-router";

import { ConnectionGate } from "@/auth/connection-gate";
import { AppShell } from "@/components/app-shell";
import { ApplicationCatalogProvider } from "@/features/applications/application-catalog";
import { ApplicationsPage } from "@/features/applications/applications-page";
import { DatasetDetail } from "@/features/datasets/dataset-detail";
import { DatasetsPage } from "@/features/datasets/datasets-page";
import { MetricsPage } from "@/features/metrics/metrics-page";
import { RunDetail } from "@/features/runs/run-detail";
import { RunsPage } from "@/features/runs/runs-page";
import { TraceDetail } from "@/features/traces/trace-detail";
import { TracesPage } from "@/features/traces/traces-page";

export function AppRoutes(): ReactNode {
  return (
    <ApplicationCatalogProvider>
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
            <Route path="runs" element={<RunsPage />} />
            <Route path="runs/:runId" element={<RunDetail />} />
            <Route path="metrics" element={<MetricsPage />} />
          </Route>
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
    </ApplicationCatalogProvider>
  );
}

function NotFound(): ReactNode {
  return (
    <main className="px-5 py-6 sm:px-8">
      <h1 className="text-xl font-semibold">Page not found</h1>
    </main>
  );
}
