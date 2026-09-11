import { expect, test } from "vitest";

import {
  formatLocalDateTime,
  parseLocalDateTime,
  readTraceFilters,
  serializeTraceFilters,
} from "@/features/traces/trace-filters";

test("serializes URL-backed trace filters and restores them", () => {
  const filters = readTraceFilters(
    new URLSearchParams(
      "q=Needle&status=error&scorer=groundedness&passed=false&range=custom&start=2026-09-01T00%3A00%3A00.000Z&end=2026-09-02T00%3A00%3A00.000Z",
    ),
    new Date("2026-09-03T00:00:00Z"),
  );

  expect(filters).toEqual({
    end: "2026-09-02T00:00:00.000Z",
    passed: false,
    q: "Needle",
    range: "custom",
    scorer: "groundedness",
    start: "2026-09-01T00:00:00.000Z",
    status: "error",
  });
  expect(serializeTraceFilters(filters)).toContain("passed=false");
});

test("drops a pass filter without a valid scorer", () => {
  const filters = readTraceFilters(
    new URLSearchParams("passed=false"),
    new Date("2026-09-03T12:00:00Z"),
  );

  expect(filters.passed).toBeUndefined();
  expect(serializeTraceFilters(filters)).not.toContain("passed");
});

test("defaults relative ranges to fixed timestamps", () => {
  const filters = readTraceFilters(new URLSearchParams(), new Date("2026-09-03T12:00:00Z"));

  expect(filters.range).toBe("24h");
  expect(filters.start).toBe("2026-09-02T12:00:00.000Z");
  expect(filters.end).toBe("2026-09-03T12:00:00.000Z");
});

test("round trips datetime-local values in the user's timezone", () => {
  const timezone = process.env["TZ"];
  process.env["TZ"] = "America/New_York";
  try {
    const instant = "2026-01-15T15:30:00.000Z";
    const local = formatLocalDateTime(instant);

    expect(local).toBe("2026-01-15T10:30");
    expect(parseLocalDateTime(local)).toBe(instant);
  } finally {
    if (timezone === undefined) delete process.env["TZ"];
    else process.env["TZ"] = timezone;
  }
});
