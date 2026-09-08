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
