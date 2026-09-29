import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";

const appId = "019d11d2-cbd3-7a5e-ae83-9b791c9329de";
const traceId = "019d11d2-cbd3-7a5e-ae83-9b791c9329aa";
const secondId = "019d11d2-cbd3-7a5e-ae83-9b791c9329ab";
const time = "2026-09-01T10:00:00Z";

type Scenario = {
  sessions: "populated" | "empty" | "error";
  traceError: boolean;
  long: boolean;
  localMode?: boolean;
};

async function mockSessionAPI(page: Page, scenario: Scenario): Promise<void> {
  await page.route("**/v1/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (scenario.localMode) {
      expect(route.request().headers()["authorization"]).toBeUndefined();
    }
    if (path === "/v1/server-info") {
      await route.fulfill({ json: { local_mode: scenario.localMode ?? false } });
      return;
    }
    if (path === "/v1/applications") {
      await route.fulfill({
        json: {
          items: [
            {
              id: appId,
              project_id: appId,
              name: scenario.long ? "Long application " + "X".repeat(90) : "Demo",
              slug: "demo",
              config: {},
              auto_score_scorers: [],
              created_at: time,
              updated_at: time,
            },
          ],
        },
      });
      return;
    }
    if (path === "/v1/sessions") {
      if (scenario.sessions === "error") {
        await route.fulfill({ status: 503, json: { title: "Storage unavailable" } });
        return;
      }
      await route.fulfill({
        json: {
          items:
            scenario.sessions === "empty"
              ? []
              : [
                  {
                    id: scenario.long ? "session-" + "x".repeat(96) : "sample-session",
                    start_time: time,
                    end_time: "2026-09-01T10:00:04Z",
                    first_operation: "Answer question",
                    last_trace_id: traceId,
                    turn_count: 2,
                  },
                ],
        },
      });
      return;
    }
    if (path === "/v1/session-turns") {
      await route.fulfill({
        json: {
          items: [
            turn(traceId, scenario.long ? "Long " + "X".repeat(2_000) : "Question one", time),
            turn(secondId, "Question two", "2026-09-01T10:00:02Z"),
          ],
        },
      });
      return;
    }
    if (path === `/v1/traces/${traceId}` || path === `/v1/traces/${secondId}`) {
      await route.fulfill(
        scenario.traceError
          ? { status: 503, json: { title: "Trace unavailable" } }
          : { json: trace(path.endsWith(secondId) ? secondId : traceId) },
      );
      return;
    }
    throw new Error(`Unexpected mocked Assay request: ${path}`);
  });
}

function turn(id: string, question: string, start: string) {
  return {
    id,
    root_name: "answer",
    status: "ok",
    start_time: start,
    end_time: "2026-09-01T10:00:04Z",
    span_count: 2,
    total_tokens: 18,
    attributes: {
      "gen_ai.input.messages": JSON.stringify([{ role: "user", content: question }]),
      "gen_ai.output.messages": JSON.stringify([{ role: "assistant", content: "In Postgres." }]),
    },
  };
}

function trace(id: string) {
  return {
    id,
    application_id: appId,
    root_name: "answer",
    status: "ok",
    span_count: 2,
    total_tokens: 18,
    start_time: time,
    end_time: "2026-09-01T10:00:04Z",
    attributes: {},
    spans: [
      {
        id: 1,
        otel_span_id: "root",
        name: "answer",
        kind: "internal",
        status_code: "ok",
        is_scorable: false,
        start_time: time,
        end_time: "2026-09-01T10:00:04Z",
        duration_ms: 4000,
        input_tokens: 0,
        output_tokens: 0,
        attributes: {},
        events: [],
        children: [
          {
            id: 2,
            otel_span_id: "model",
            name: "generate",
            kind: "client",
            status_code: "ok",
            is_scorable: true,
            start_time: "2026-09-01T10:00:01Z",
            end_time: "2026-09-01T10:00:03Z",
            duration_ms: 2000,
            input_tokens: 12,
            output_tokens: 6,
            events: [],
            children: [],
            attributes: {
              "gen_ai.request.model": "local-model",
              "gen_ai.input.messages": JSON.stringify([
                { role: "user", content: "Prior model context" },
              ]),
              "gen_ai.output.messages": JSON.stringify([
                { role: "assistant", content: "Model answer" },
              ]),
              "gen_ai.retrieval.documents": JSON.stringify([
                { id: "postgres", text: "Assay stores traces in PostgreSQL." },
              ]),
            },
          },
        ],
      },
    ],
    scores: [
      {
        id: 10,
        span_id: 2,
        scorer: "groundedness",
        passed: true,
        value: 0.93,
        threshold: 0.7,
        rationale: "Supported by retrieval",
        details: {},
        judge_provider: "local",
        judge_model: "mistral",
        judge_tokens: 20,
        prompt_template_id: "groundedness-v1",
        created_at: time,
      },
    ],
  };
}

for (const width of [320, 1440]) {
  test(`local mode opens sessions without stored credentials at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.addInitScript(() => localStorage.setItem("assay.admin-token.v1", "stale-token"));
    await mockSessionAPI(page, {
      sessions: "populated",
      traceError: false,
      long: false,
      localMode: true,
    });
    await page.goto(`/apps/${appId}/sessions`);
    await expect(page.getByRole("link", { name: "Answer question" })).toBeVisible();
    await expect(page.getByText("Local mode", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Disconnect" })).toHaveCount(0);
    await expect(page.getByLabel("Admin token")).toHaveCount(0);
    expect(await page.evaluate(() => localStorage.getItem("assay.admin-token.v1"))).toBeNull();
    expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(
      false,
    );
    expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  });
}

for (const theme of ["dark", "light"] as const) {
  for (const width of [360, 1440]) {
    test(`sessions and inspector pass axe at ${width}px in ${theme} mode`, async ({ page }) => {
      await page.setViewportSize({ width, height: 900 });
      await page.addInitScript((preference) => {
        localStorage.setItem("assay.admin-token.v1", "synthetic-token");
        localStorage.setItem("assay.theme.v1", preference);
      }, theme);
      await mockSessionAPI(page, { sessions: "populated", traceError: false, long: false });
      await page.goto(`/apps/${appId}/sessions`);
      await expect(page.getByRole("link", { name: "Answer question" })).toBeVisible();
      await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      await page.getByRole("link", { name: "Answer question" }).click();
      const transcript = page.getByRole("list", { name: "Session turns" });
      await expect(transcript.getByText("Question one")).toBeVisible();
      await expect(transcript.getByText("Prior model context")).toHaveCount(0);
      await transcript
        .getByRole("button", { name: /Inspect source for assistant message/ })
        .first()
        .click();
      const inspector = page.getByRole("complementary", { name: "Selected trace inspector" });
      await expect(inspector.getByRole("region", { name: "Source scores" })).toContainText("0.93");
      await inspector.getByText("Model calls and captured context").click();
      await expect(inspector.getByText("Prior model context")).toBeVisible();
      await page.getByRole("button", { name: "Timeline" }).click();
      const timeline = page.getByRole("region", { name: "Session timeline" });
      await timeline.getByRole("button", { name: "Zoom in" }).click();
      await expect(timeline.getByText("2×")).toBeVisible();
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(
        false,
      );
    });
  }
}

test("approved composition and controls stay aligned on desktop and mobile", async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem("assay.admin-token.v1", "synthetic-token");
    localStorage.setItem("assay.theme.v1", "dark");
  });
  await mockSessionAPI(page, { sessions: "populated", traceError: false, long: false });
  for (const width of [1440, 390, 320]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(`/apps/${appId}/sessions/detail?session_id=sample-session`);
    const thread = page.getByRole("region", { name: "Session conversation" });
    const recent = page.getByRole("complementary", { name: "Recent sessions" });
    const inspector = page.getByRole("complementary", { name: "Selected trace inspector" });
    await expect(thread).toBeVisible();
    await expect(page.getByRole("region", { name: "Session summary" })).toContainText("4");
    const question = thread.getByRole("button", { name: /Question one/ });
    await question.click();
    await expect(inspector.getByText("Tokens used")).toBeVisible();
    await expect(thread.getByRole("region", { name: "Span timing" })).toBeVisible();
    const threadBox = await thread.boundingBox();
    const inspectorBox = await inspector.boundingBox();
    if (!threadBox || !inspectorBox) throw new Error("Missing workspace panels");
    if (width === 1440) {
      await expect(recent).toBeVisible();
      const recentBox = await recent.boundingBox();
      if (!recentBox) throw new Error("Missing recent sessions");
      expect(recentBox.x + recentBox.width).toBeLessThan(threadBox.x);
      expect(threadBox.x + threadBox.width).toBeLessThan(inspectorBox.x);
      expect(Math.abs(recentBox.y - threadBox.y)).toBeLessThan(2);
      expect(Math.abs(inspectorBox.y - threadBox.y)).toBeLessThan(2);
    } else {
      await expect(recent).toBeHidden();
      expect(inspectorBox.y).toBeGreaterThanOrEqual(threadBox.y + threadBox.height);
    }
    const userColor = await question.evaluate(
      (element) => getComputedStyle(element).backgroundColor,
    );
    const answerColor = await thread
      .getByRole("button", { name: /In Postgres/ })
      .first()
      .evaluate((element) => getComputedStyle(element).backgroundColor);
    expect(userColor).not.toBe(answerColor);
    const controls = page.locator("header").first().locator("select, button");
    const boxes = await controls.evaluateAll((elements) =>
      elements.map((element) => {
        const box = element.getBoundingClientRect();
        return { x: box.x, y: box.y, right: box.right, height: box.height, width: box.width };
      }),
    );
    for (const box of boxes.filter((box) => box.width > 0)) {
      expect(box.x).toBeGreaterThanOrEqual(0);
      expect(box.right).toBeLessThanOrEqual(width);
      expect(box.height).toBeGreaterThanOrEqual(36);
      expect(box.width).toBeGreaterThanOrEqual(44);
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(
      false,
    );
    expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  }
});

test("toolbar buttons stay aligned with long application names and system theme", async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem("assay.admin-token.v1", "synthetic-token");
    localStorage.setItem("assay.theme.v1", "system");
  });
  await mockSessionAPI(page, { sessions: "populated", traceError: false, long: true });
  for (const width of [320, 390, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(`/apps/${appId}/sessions`);
    await expect(page.getByRole("link", { name: "Answer question" })).toBeVisible();
    const controls = await page
      .locator("header")
      .first()
      .locator("button, select")
      .evaluateAll((elements) =>
        elements
          .map((element) => {
            const box = element.getBoundingClientRect();
            return {
              x: box.x,
              y: box.y,
              right: box.right,
              width: box.width,
              height: box.height,
              font: parseFloat(getComputedStyle(element).fontSize),
            };
          })
          .filter((box) => box.width > 0),
      );
    for (const [index, control] of controls.entries()) {
      expect(control.right).toBeLessThanOrEqual(width);
      expect(control.x).toBeGreaterThanOrEqual(0);
      expect(control.font).toBeLessThanOrEqual(14);
      expect(control.height).toBe(width < 768 ? 44 : 36);
      const previous = controls[index - 1];
      if (previous) {
        expect(control.x).toBeGreaterThanOrEqual(previous.right);
        expect(control.y).toBe(previous.y);
      }
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(
      false,
    );
    if (width < 768) {
      await page.getByRole("button", { name: "Open navigation" }).click();
      await expect(page.getByRole("dialog")).toBeVisible();
      expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
      await page.getByRole("dialog").getByRole("link", { name: "Sessions", exact: true }).click();
      await expect(page.getByRole("dialog")).toHaveCount(0);
    }
  }
});

test("timeline is keyboard scrollable at a 200%-equivalent CSS viewport", async ({ page }) => {
  await page.setViewportSize({ width: 640, height: 900 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.addInitScript(() => localStorage.setItem("assay.admin-token.v1", "synthetic-token"));
  await mockSessionAPI(page, { sessions: "populated", traceError: false, long: false });
  await page.goto(`/apps/${appId}/sessions/detail?session_id=sample-session`);
  await page.getByRole("button", { name: "Timeline" }).click();
  const timeline = page.getByRole("region", { name: "Session timeline" });
  await timeline.getByRole("button", { name: "Zoom in" }).click();
  await timeline.getByRole("button", { name: "Zoom in" }).click();
  const chart = page.getByRole("region", { name: "Scrollable timing chart" });
  await chart.focus();
  await page.keyboard.press("ArrowRight");
  await expect.poll(() => chart.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0);
  await page.keyboard.press("Tab");
  await expect(timeline.getByRole("button", { name: /Turn 1 · answer/ })).toBeFocused();
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
});

test("long text, empty/error states and trace retry remain usable at mobile width", async ({
  page,
}) => {
  await page.setViewportSize({ width: 360, height: 800 });
  await page.addInitScript(() => localStorage.setItem("assay.admin-token.v1", "synthetic-token"));
  const scenario: Scenario = { sessions: "empty", traceError: false, long: true };
  await mockSessionAPI(page, scenario);
  await page.goto(`/apps/${appId}/sessions`);
  await expect(page.getByText("Your traces are still here.")).toBeVisible();
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  scenario.sessions = "error";
  await page.getByRole("button", { name: "Refresh" }).click();
  await expect(page.getByRole("alert")).toContainText("Storage unavailable");
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
  scenario.sessions = "populated";
  await page.getByRole("button", { name: "Refresh" }).click();
  await page.getByRole("link", { name: "Answer question" }).click();
  await expect(page.getByRole("list", { name: "Session turns" }).getByText(/X{100}/)).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
  scenario.traceError = true;
  await page
    .getByRole("button", { name: /Inspect source for assistant message/ })
    .first()
    .click();
  const inspector = page.getByRole("complementary", { name: "Selected trace inspector" });
  await expect(inspector.getByRole("alert")).toContainText("Trace unavailable");
  scenario.traceError = false;
  await inspector.getByRole("button", { name: "Refresh source trace" }).click();
  await expect(inspector.getByRole("region", { name: "Source scores" })).toContainText("0.93");
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([]);
});
