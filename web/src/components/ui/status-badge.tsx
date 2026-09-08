import type { ReactNode } from "react";

export type StatusTone = "success" | "danger" | "warning" | "neutral" | "accent";

export type StatusBadgeProps = {
  tone: StatusTone;
  children: ReactNode;
  className?: string;
};

const toneClasses: Record<StatusTone, string> = {
  success: "bg-success/10 text-success ring-1 ring-inset ring-success/30",
  danger: "bg-danger/10 text-danger ring-1 ring-inset ring-danger/30",
  warning: "bg-warning/10 text-warning ring-1 ring-inset ring-warning/30",
  neutral: "bg-surface text-muted ring-1 ring-inset ring-line",
  accent: "bg-accent/10 text-accent ring-1 ring-inset ring-accent/30",
};

export function StatusBadge({ tone, children, className }: StatusBadgeProps): React.ReactElement {
  return (
    <span
      className={[
        "inline-flex items-center gap-1 rounded-full px-2 py-0.5",
        "text-xs font-medium whitespace-nowrap",
        toneClasses[tone],
        className,
      ].join(" ")}
    >
      {children}
    </span>
  );
}
