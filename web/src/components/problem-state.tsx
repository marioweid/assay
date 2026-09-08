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
