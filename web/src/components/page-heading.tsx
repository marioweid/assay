import type { ReactNode } from "react";

export type PageHeadingProps = {
  title: string;
  description?: string;
};

export function PageHeading({ title, description }: PageHeadingProps): ReactNode {
  return (
    <header className="mb-6">
      <h1 className="text-2xl font-semibold tracking-tight text-ink">{title}</h1>
      {description !== undefined && (
        <p className="mt-1 max-w-2xl text-sm text-muted">{description}</p>
      )}
    </header>
  );
}
