import type { ScoreResponse } from "@/api/generated/types.gen";

import { JsonView } from "@/components/json-view";
import { StatusBadge } from "@/components/ui/status-badge";

type ScoreResultProps = {
  score: ScoreResponse;
};

export function ScoreResult({ score }: ScoreResultProps) {
  return (
    <article className="border border-line bg-surface p-4">
      <div className="flex flex-wrap items-center gap-3">
        <h3 className="font-semibold">{score.scorer}</h3>
        <StatusBadge tone={score.passed ? "success" : "danger"}>
          {score.passed ? "Passed" : "Failed"}
        </StatusBadge>
        <strong className="font-mono text-lg">{score.value}</strong>
        <span className="text-sm text-muted">Threshold {score.threshold}</span>
      </div>
      <p className="mt-3 text-sm leading-6">{score.rationale}</p>
      <dl className="mt-4 grid gap-2 text-xs text-muted sm:grid-cols-2">
        <div>
          <dt>Model</dt>
          <dd className="text-ink">
            {score.judge_provider} / {score.judge_model}
          </dd>
        </div>
        <div>
          <dt>Prompt</dt>
          <dd className="text-ink">{score.prompt_template_id}</dd>
        </div>
        <div>
          <dt>Judge tokens</dt>
          <dd className="text-ink">{score.judge_tokens}</dd>
        </div>
      </dl>
      {Object.keys(score.details).length > 0 && (
        <div className="mt-4">
          <JsonView value={score.details} />
        </div>
      )}
    </article>
  );
}
