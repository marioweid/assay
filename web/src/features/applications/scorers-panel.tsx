import { useState } from "react";

import { Problem } from "@/api/errors";
import { putScorerConfig } from "@/api/generated/sdk.gen";
import type { ScorerConfigResponse } from "@/api/generated/types.gen";
import { Button } from "@/components/ui/button";
import { fieldControlClass } from "@/components/ui/field";

const scorers = ["groundedness", "correctness"] as const;

export function ScorersPanel({
  applicationID,
  configs,
  onSaved,
}: {
  applicationID: string;
  configs: ScorerConfigResponse[];
  onSaved: () => void;
}): React.ReactElement {
  return (
    <section className="space-y-4 border-t border-line pt-6">
      <div>
        <h2 className="text-lg font-semibold">Evaluation</h2>
        <p className="mt-1 text-sm text-muted">
          Process defaults are overridden by project settings, then these scorer settings.
        </p>
      </div>
      {scorers.map((scorer) => (
        <ScorerForm
          applicationID={applicationID}
          config={configs.find((item) => item.scorer === scorer)}
          key={scorer}
          onSaved={onSaved}
          scorer={scorer}
        />
      ))}
    </section>
  );
}

function ScorerForm({
  applicationID,
  config,
  scorer,
  onSaved,
}: {
  applicationID: string;
  config: ScorerConfigResponse | undefined;
  scorer: (typeof scorers)[number];
  onSaved: () => void;
}): React.ReactElement {
  const label = scorer.charAt(0).toUpperCase() + scorer.slice(1);
  const [enabled, setEnabled] = useState(config?.enabled ?? true);
  const [threshold, setThreshold] = useState(String(config?.threshold ?? 0.5));
  const [promptTemplateID, setPromptTemplateID] = useState(config?.prompt_template_id ?? "");
  const [baseURL, setBaseURL] = useState(config?.judge_config?.base_url ?? "");
  const [model, setModel] = useState(config?.judge_config?.model ?? "");
  const [apiKey, setAPIKey] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function save(): Promise<void> {
    const parsedThreshold = Number(threshold);
    if (!Number.isFinite(parsedThreshold) || parsedThreshold < 0 || parsedThreshold > 1) {
      setError("Threshold must be between 0 and 1.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await putScorerConfig({
        body: {
          enabled,
          ...(baseURL === "" && model === "" && apiKey === ""
            ? {}
            : {
                judge_config: {
                  ...(apiKey === "" ? {} : { api_key: apiKey }),
                  base_url: baseURL,
                  model,
                },
              }),
          prompt_template_id: promptTemplateID,
          threshold: parsedThreshold,
        },
        path: { application_id: applicationID, scorer },
        throwOnError: true,
      });
      setAPIKey("");
      onSaved();
    } catch (reason) {
      setError(
        reason instanceof Problem ? (reason.detail ?? reason.title) : "Unable to save scorer",
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <fieldset className="grid max-w-3xl gap-3 rounded-md border border-line p-4">
      <legend className="px-1 font-medium">{label}</legend>
      <label className="flex items-center gap-2 text-sm">
        <input
          checked={enabled}
          onChange={(event) => setEnabled(event.target.checked)}
          type="checkbox"
        />{" "}
        Enabled for evaluation runs
      </label>
      <label className="text-sm">
        <span className="font-medium">{label} threshold</span>
        <input
          className={`${fieldControlClass} mt-1`}
          max="1"
          min="0"
          onChange={(event) => setThreshold(event.target.value)}
          step="any"
          type="number"
          value={threshold}
        />
      </label>
      <label className="text-sm">
        <span className="font-medium">Prompt template ID</span>
        <input
          className={`${fieldControlClass} mt-1`}
          onChange={(event) => setPromptTemplateID(event.target.value)}
          value={promptTemplateID}
        />
      </label>
      <label className="text-sm">
        <span className="font-medium">Judge base URL</span>
        <input
          className={`${fieldControlClass} mt-1`}
          onChange={(event) => setBaseURL(event.target.value)}
          type="url"
          value={baseURL}
        />
      </label>
      <label className="text-sm">
        <span className="font-medium">Judge model</span>
        <input
          className={`${fieldControlClass} mt-1`}
          onChange={(event) => setModel(event.target.value)}
          value={model}
        />
      </label>
      <label className="text-sm">
        <span className="font-medium">Replace judge API key</span>
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
      <div>
        <Button disabled={saving} onClick={() => void save()} variant="primary">
          Save {scorer} scorer
        </Button>
      </div>
    </fieldset>
  );
}
