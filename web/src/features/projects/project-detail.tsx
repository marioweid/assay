import type { ReactNode } from "react";

import { PageHeading } from "@/components/page-heading";

export function ProjectDetail(): ReactNode {
  return (
    <main className="min-h-screen bg-canvas px-5 py-6 text-ink sm:px-8">
      <PageHeading title="Project" />
    </main>
  );
}
