import { useState } from "react";

import { Problem } from "@/api/errors";
import { updateProject } from "@/api/generated/sdk.gen";
import type { JudgeConfigResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { fieldControlClass } from "@/components/ui/field";

export function JudgeConfigForm({
  projectID,
  judgeConfig,
  onSaved,
}: {
  projectID: string;
  judgeConfig?: JudgeConfigResponse;
  onSaved: () => void;
}): React.ReactElement {
  const [baseURL, setBaseURL] = useState(judgeConfig?.base_url ?? "");
  const [model, setModel] = useState(judgeConfig?.model ?? "");
  const [apiKey, setAPIKey] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function save(): Promise<void> {
    setSaving(true);
    setError(null);
    try {
      await updateProject({
        body: {
          judge_config: { ...(apiKey === "" ? {} : { api_key: apiKey }), base_url: baseURL, model },
        },
        path: { id: projectID },
        throwOnError: true,
      });
      setAPIKey("");
      onSaved();
    } catch (reason) {
      setError(
        reason instanceof Problem
          ? (reason.detail ?? reason.title)
          : "Unable to save judge settings",
      );
    } finally {
      setSaving(false);
    }
  }

  async function clear(): Promise<void> {
    setSaving(true);
    setError(null);
    try {
      await updateProject({
        body: { clear_judge_config: true },
        path: { id: projectID },
        throwOnError: true,
      });
      onSaved();
    } catch (reason) {
      setError(
        reason instanceof Problem
          ? (reason.detail ?? reason.title)
          : "Unable to clear judge settings",
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <section className="mt-8 space-y-4">
      <div>
        <h2 className="text-lg font-semibold">Project judge override</h2>
        <p className="mt-1 text-sm text-muted">
          Overrides process defaults; scorer settings take precedence.
        </p>
      </div>
      <div className="grid max-w-3xl gap-3">
        <label className="text-sm">
          <span className="font-medium">Base URL</span>
          <input
            className={`${fieldControlClass} mt-1`}
            onChange={(event) => setBaseURL(event.target.value)}
            type="url"
            value={baseURL}
          />
        </label>
        <label className="text-sm">
          <span className="font-medium">Model</span>
          <input
            className={`${fieldControlClass} mt-1`}
            onChange={(event) => setModel(event.target.value)}
            value={model}
          />
        </label>
        <label className="text-sm">
          <span className="font-medium">Replace API key</span>
          <input
            className={`${fieldControlClass} mt-1`}
            autoComplete="new-password"
            onChange={(event) => setAPIKey(event.target.value)}
            type="password"
            value={apiKey}
          />
          <span className="mt-1 block text-xs text-muted">
            Leave blank to preserve the saved key.
          </span>
        </label>
        {error !== null && (
          <p className="text-sm text-danger" role="alert">
            {error}
          </p>
        )}
        <div className="flex gap-3">
          <Button disabled={saving} onClick={() => void save()} variant="primary">
            Save judge settings
          </Button>
          {judgeConfig !== undefined && (
            <Button disabled={saving} onClick={() => void clear()} variant="danger">
              Clear override
            </Button>
          )}
        </div>
      </div>
    </section>
  );
}
