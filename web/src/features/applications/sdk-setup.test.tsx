import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";

import { SDKSetup } from "@/features/applications/sdk-setup";

test("SDK setup targets Assay, never the application's generation endpoint", () => {
  render(
    <MemoryRouter>
      <SDKSetup
        application={{
          id: "app",
          project_id: "project",
          name: "Demo",
          slug: "demo",
          config: {},
          auto_score_scorers: [],
          created_at: "",
          updated_at: "",
          target_endpoint: {
            url: "https://target.example/chat",
            method: "POST",
            has_secret: false,
            timeout_ms: 1000,
            response_mapping: { output: "answer" },
          },
        }}
      />
    </MemoryRouter>,
  );
  expect(screen.getByText(/export ASSAY_ENDPOINT=/)).toHaveTextContent(
    `export ASSAY_ENDPOINT=${window.location.origin}`,
  );
  expect(screen.queryByText(/https:\/\/target.example/)).not.toBeInTheDocument();
});
