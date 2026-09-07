import type { ScoreResponse } from "@/api/generated/types.gen";

import { JsonView } from "@/components/json-view";

type ScoreResultProps = {
  score: ScoreResponse;
};

export function ScoreResult({ score }: ScoreResultProps) {
  return (
    <article className="border border-line bg-white p-4">
      <div className="flex flex-wrap items-center gap-3">
        <h3 className="font-semibold">{score.scorer}</h3>
        <span
          className={score.passed ? "bg-emerald-100 text-emerald-800" : "bg-red-100 text-red-800"}
        >
          {score.passed ? "Passed" : "Failed"}
        </span>
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
