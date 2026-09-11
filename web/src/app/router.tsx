import type { ReactNode } from "react";
import { Navigate, Route, Routes } from "react-router";

import { ThemeProvider } from "@/app/theme-provider";
import { ConnectionGate } from "@/auth/connection-gate";
import { AppShell } from "@/components/app-shell";
import { PageHeading } from "@/components/page-heading";
import { ApplicationCatalogProvider } from "@/features/applications/application-catalog";
import { ApplicationSettings } from "@/features/applications/application-settings";
import { ApplicationsPage } from "@/features/applications/applications-page";
import { DatasetDetail } from "@/features/datasets/dataset-detail";
import { DatasetsPage } from "@/features/datasets/datasets-page";
import { MetricsPage } from "@/features/metrics/metrics-page";
import { ProjectDetail } from "@/features/projects/project-detail";
import { ProjectsPage } from "@/features/projects/projects-page";
import { RunDetail } from "@/features/runs/run-detail";
import { RunsPage } from "@/features/runs/runs-page";
import { TraceDetail } from "@/features/traces/trace-detail";
import { TracesPage } from "@/features/traces/traces-page";

export function AppRoutes(): ReactNode {
  return (
    <ThemeProvider>
      <ApplicationCatalogProvider>
        <Routes>
          <Route element={<ConnectionGate />}>
            <Route path="/" element={<Navigate replace to="/apps" />} />
            <Route path="/apps" element={<ApplicationsPage />} />
            <Route path="/projects" element={<ProjectsPage />} />
            <Route path="/projects/:projectId" element={<ProjectDetail />} />
            <Route path="/apps/:appId" element={<AppShell />}>
              <Route index element={<Navigate replace to="traces" />} />
              <Route path="traces" element={<TracesPage />} />
              <Route path="traces/:traceId" element={<TraceDetail />} />
              <Route path="datasets" element={<DatasetsPage />} />
              <Route path="datasets/:datasetId" element={<DatasetDetail />} />
              <Route path="runs" element={<RunsPage />} />
              <Route path="runs/:runId" element={<RunDetail />} />
              <Route path="metrics" element={<MetricsPage />} />
              <Route path="settings" element={<ApplicationSettings />} />
            </Route>
            <Route path="*" element={<NotFound />} />
          </Route>
        </Routes>
      </ApplicationCatalogProvider>
    </ThemeProvider>
  );
}

function NotFound(): ReactNode {
  return (
    <main className="px-5 py-6 sm:px-8">
      <PageHeading title="Page not found" />
    </main>
  );
}
