import { test, expect } from "vitest";
import { dur, clock, delta, pct, dayTitle, isoDate, parseDate, addDays } from "./format";

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
