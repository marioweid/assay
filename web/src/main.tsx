import { createRoot } from "react-dom/client";

import "./styles.css";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error('Assay web root element "#root" was not found');
}

createRoot(rootElement).render(
  <main className="min-h-screen bg-canvas px-6 py-8 text-ink">
    <h1 className="text-xl font-semibold tracking-tight">Assay</h1>
    <p className="mt-2 text-sm text-muted">Evaluation workbench</p>
  </main>,
);
