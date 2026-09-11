import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { useParams } from "react-router";

import { Problem } from "@/api/errors";
import { listScorerConfigs, updateApplication } from "@/api/generated/sdk.gen";
import type { ScorerConfigResponse } from "@/api/generated/types.gen";
import { LoadingState } from "@/components/loading-state";
import { PageHeading } from "@/components/page-heading";
import { ProblemState } from "@/components/problem-state";
import { EndpointForm } from "@/features/applications/endpoint-form";
import { useApplicationCatalog } from "@/features/applications/application-catalog";
import { ScorersPanel } from "@/features/applications/scorers-panel";
import { SDKSetup } from "@/features/applications/sdk-setup";

export function ApplicationSettings(): ReactNode {
  const { appId = "" } = useParams();
  const {
    applications,
    error: catalogError,
    loading: loadingApplications,
    refresh: refreshCatalog,
  } = useApplicationCatalog();
  const application = applications.find((item) => item.id === appId);
  const [configs, setConfigs] = useState<ScorerConfigResponse[]>([]);
  const [loadingConfigs, setLoadingConfigs] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [savingAutoScore, setSavingAutoScore] = useState(false);

  useEffect(() => {
    if (application === undefined) return;
    const controller = new AbortController();
    setLoadingConfigs(true);
    setError(null);
    void listScorerConfigs({
      path: { application_id: application.id },
      signal: controller.signal,
      throwOnError: true,
    })
      .then((response) => {
        if (!controller.signal.aborted) setConfigs(response.data.items ?? []);
      })
      .catch((reason) => {
        if (!controller.signal.aborted)
          setError(reason instanceof Problem ? reason.title : "Unable to load scorer settings");
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingConfigs(false);
      });
    return () => controller.abort();
  }, [application, refreshKey]);

  if (application === undefined) {
    if (loadingApplications) return <LoadingState label="Loading application settings" />;
    if (catalogError !== null) {
      return (
        <ProblemState
          detail={catalogError}
          onRetry={() => void refreshCatalog()}
          title="Applications unavailable"
        />
      );
    }
    return <ProblemState title="Application not found" />;
  }
  if (loadingConfigs) return <LoadingState label="Loading application settings" />;
  if (error !== null)
    return (
      <ProblemState
        detail={error}
        onRetry={() => setRefreshKey((value) => value + 1)}
        title="Settings unavailable"
      />
    );
  const currentApplication = application;

  async function saveAutoScore(scorer: string, checked: boolean): Promise<void> {
    const current = currentApplication.auto_score_scorers ?? [];
    const autoScoreScorers = checked
      ? [...new Set([...current, scorer])]
      : current.filter((item) => item !== scorer);
    setSavingAutoScore(true);
    try {
      await updateApplication({
        body: { auto_score_scorers: autoScoreScorers },
        path: { id: currentApplication.id },
        throwOnError: true,
      });
      await refreshCatalog();
    } finally {
      setSavingAutoScore(false);
    }
  }

  const refresh = (): void => {
    setRefreshKey((value) => value + 1);
    void refreshCatalog();
  };
  return (
    <section>
      <PageHeading
        description="Configure target generation and scoring without exposing saved credentials."
        title="Application settings"
      />
      <AutoScoreControls
        autoScorers={currentApplication.auto_score_scorers ?? []}
        disabled={savingAutoScore}
        onChange={saveAutoScore}
      />
      <EndpointForm
        applicationID={currentApplication.id}
        {...(currentApplication.target_endpoint === undefined
          ? {}
          : { endpoint: currentApplication.target_endpoint })}
        onSaved={refresh}
      />
      <ScorersPanel applicationID={currentApplication.id} configs={configs} onSaved={refresh} />
      <SDKSetup application={currentApplication} />
    </section>
  );
}

function AutoScoreControls({
  autoScorers,
  disabled,
  onChange,
}: {
  autoScorers: string[];
  disabled: boolean;
  onChange: (scorer: string, checked: boolean) => Promise<void>;
}): React.ReactElement {
  return (
    <section className="space-y-3">
      <h2 className="text-lg font-semibold">Automatic scoring</h2>
      <p className="text-sm text-muted">
        These controls enqueue scoring for new traces; scorer enablement separately controls
        evaluation runs.
      </p>
      {["groundedness", "correctness"].map((scorer) => (
        <label className="mr-5 inline-flex items-center gap-2 text-sm" key={scorer}>
          <input
            checked={autoScorers.includes(scorer)}
            disabled={disabled}
            onChange={(event) => void onChange(scorer, event.target.checked)}
            type="checkbox"
          />
          {scorer.charAt(0).toUpperCase() + scorer.slice(1)}
        </label>
      ))}
    </section>
  );
}
