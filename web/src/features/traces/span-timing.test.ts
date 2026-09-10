import { layoutSpanTiming, parseTimestampNs } from "@/features/traces/span-timing";

test("positions overlapping calls by time rather than sequential duration", () => {
  const layout = layoutSpanTiming([
    { key: "root@t", startTime: "2026-09-08T00:00:00Z", endTime: "2026-09-08T00:00:01Z" },
    { key: "a@t", startTime: "2026-09-08T00:00:00.1Z", endTime: "2026-09-08T00:00:00.6Z" },
    { key: "b@t", startTime: "2026-09-08T00:00:00.2Z", endTime: "2026-09-08T00:00:00.7Z" },
  ]);

  expect(layout.rows[1]).toMatchObject({ leftPercent: 10, widthPercent: 50 });
  expect(layout.rows[2]).toMatchObject({ leftPercent: 20, widthPercent: 50 });
});

test("keeps fractional milliseconds", () => {
  const start = parseTimestampNs("2026-09-08T00:00:00.000100Z");
  const end = parseTimestampNs("2026-09-08T00:00:00.000400Z");

  expect(start).not.toBeNull();
  expect(end).not.toBeNull();
  if (start !== null && end !== null) expect(Number(end - start) / 1e6).toBeCloseTo(0.3);
});

test("keeps invalid and zero-duration timing finite without mutating input", () => {
  const spans = [
    { key: "bad@t" as const, startTime: "now", endTime: "later" },
    {
      key: "zero@t" as const,
      startTime: "2026-09-08T00:00:00+02:00",
      endTime: "2026-09-08T00:00:00+02:00",
    },
    {
      key: "backwards@t" as const,
      startTime: "2026-09-08T00:00:01Z",
      endTime: "2026-09-08T00:00:00Z",
    },
  ];
  const original = structuredClone(spans);
  const layout = layoutSpanTiming(spans);

  expect(spans).toEqual(original);
  expect(layout.rows.map((row) => row.invalid)).toEqual([true, false, true]);
  for (const row of layout.rows) {
    expect(row.leftPercent).toBeGreaterThanOrEqual(0);
    expect(row.leftPercent + row.widthPercent).toBeLessThanOrEqual(100);
  }
  expect(layout.rows[1]).toMatchObject({ durationMs: 0, leftPercent: 0, widthPercent: 0 });
});

test("rejects invalid calendar dates and preserves timezone offsets", () => {
  expect(parseTimestampNs("2026-02-29T00:00:00Z")).toBeNull();
  expect(parseTimestampNs("2026-09-08T00:00:00+02:00")).toBe(
    parseTimestampNs("2026-09-07T22:00:00Z"),
  );
});
