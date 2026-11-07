import { test, expect } from "vitest";
import { dur, clock, delta, pct, dayTitle, isoDate, parseDate, addDays, entries, doneText } from "./format";

test("dur", () => {
  expect(dur(8_640_000)).toBe("2h24");
  expect(dur(2_700_000)).toBe("45m");
  expect(dur(0)).toBe("0m");
  expect(dur(3_600_000)).toBe("1h00");
});
test("clock", () => {
  expect(clock(8_640_000)).toBe("2:24");
  expect(clock(300_000)).toBe("0:05");
});
test("delta", () => {
  expect(delta(2_880_000)).toBe("+48m");
  expect(delta(-1_380_000)).toBe("−23m");
});
test("pct", () => {
  expect(pct(1, 4)).toBe("25 %");
  expect(pct(1, 0)).toBe("0 %");
});
test("dates", () => {
  const d = parseDate("2026-11-05");
  expect(isoDate(d)).toBe("2026-11-05");
  expect(dayTitle(d)).toBe("Thu 5 Nov");
  expect(isoDate(addDays(d, -5))).toBe("2026-10-31");
});

test("entries counts in the singular for one", () => {
  expect(entries(1)).toBe("1 entry");
  expect(entries(0)).toBe("0 entries");
  expect(entries(14)).toBe("14 entries");
});

test("doneText names the range a timeline label covers, else the count", () => {
  const d = { label: "Research", spans: 1, ms: 960_000 };
  expect(doneText(d)).toBe("Research · 1 entry, 16m");
  expect(doneText({ ...d, ms: 0 }, "14:41–14:53 (12m)")).toBe("Labelled 14:41–14:53 (12m) · Research");
});
