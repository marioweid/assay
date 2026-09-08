import type { ReactNode } from "react";

export const fieldControlClass = [
  "block w-full rounded-md border border-line bg-surface px-3 py-2",
  "text-sm text-ink placeholder:text-muted",
].join(" ");

export type FieldProps = {
  label: string;
  children: ReactNode;
  hint?: string;
  error?: string | null;
};

export function Field({ label, children, hint, error }: FieldProps): React.ReactElement {
  const errorMessage = error != null && error !== "" ? error : undefined;
  return (
    <div className="space-y-1">
      <div className="block text-sm font-medium text-ink">{label}</div>
      {children}
      {hint !== undefined && <p className="text-xs text-muted">{hint}</p>}
      {errorMessage !== undefined && (
        <p className="text-xs text-danger" role="alert">
          {errorMessage}
        </p>
      )}
    </div>
  );
}
