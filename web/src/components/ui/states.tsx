import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";

export type ProblemStateProps = {
  title: string;
  detail?: string;
  onRetry?: () => void;
};

export function ProblemState({ title, detail, onRetry }: ProblemStateProps): React.ReactElement {
  return (
    <div className="mx-auto max-w-md rounded-lg border border-danger/30 bg-surface p-6 text-center">
      <h2 className="text-lg font-semibold text-ink">{title}</h2>
      {detail !== undefined && <p className="mt-2 text-sm text-muted">{detail}</p>}
      {onRetry !== undefined && (
        <Button className="mt-4" variant="primary" onClick={onRetry}>
          Try again
        </Button>
      )}
    </div>
  );
}

export type EmptyStateProps = {
  title: string;
  description: string;
  action?: ReactNode;
};

export function EmptyState({ title, description, action }: EmptyStateProps): React.ReactElement {
  return (
    <div className="mx-auto max-w-md rounded-lg border border-line bg-surface p-6 text-center">
      <h3 className="text-base font-semibold text-ink">{title}</h3>
      <p className="mt-1 text-sm text-muted">{description}</p>
      {action !== undefined && <div className="mt-4 flex justify-center">{action}</div>}
    </div>
  );
}

export function LoadingState({ label }: { label: string }): React.ReactElement {
  return (
    <div aria-busy="true" className="space-y-3" role="status">
      <div className="h-5 w-2/5 animate-pulse rounded bg-line/70" />
      <div className="h-5 w-3/5 animate-pulse rounded bg-line/50" />
      <div className="h-5 w-1/2 animate-pulse rounded bg-line/70" />
      <span className="sr-only">{label}</span>
    </div>
  );
}
