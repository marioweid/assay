import { Link } from "react-router";

import type { ApplicationResponse } from "@/api/generated/types.gen";

export function SDKSetup({
  application,
}: {
  application: ApplicationResponse;
}): React.ReactElement {
  const endpoint = application.target_endpoint?.url ?? "http://localhost:8080";
  return (
    <section className="space-y-3 border-t border-line pt-6">
      <div>
        <h2 className="text-lg font-semibold">SDK setup</h2>
        <p className="mt-1 text-sm text-muted">
          Capture is optional and may include sensitive content.
        </p>
      </div>
      <pre className="overflow-x-auto rounded-md border border-line bg-canvas p-4 text-sm">
        <code>{`export ASSAY_ENDPOINT=${endpoint}\nexport ASSAY_API_KEY=asy_your_key\nexport ASSAY_APPLICATION=${application.slug}\n\nimport assay\n\nassay.init(capture=True)\n\n@assay.trace\ndef answer(question: str) -> str:\n    return "Assay evaluates AI systems."\n\nanswer("What is Assay?")\nassay.shutdown()`}</code>
      </pre>
      <p className="text-sm text-muted">
        Create an ingest key explicitly in{" "}
        <Link className="text-accent hover:underline" to={`/projects/${application.project_id}`}>
          project API keys
        </Link>
        .
      </p>
    </section>
  );
}
