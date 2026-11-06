import { test, expect } from "vitest";
import { atClock, clockOf, rangeError, snapAt } from "./range";
import { hhmm, parseDate } from "./format";

const day = "2026-11-05";
const d0 = parseDate(day).getTime();
const H = 3_600_000;

test("atClock reads HH:MM on the day", () => {
  expect(atClock(day, "09:05")).toBe(d0 + 9 * H + 5 * 60_000);
  expect(atClock(day, " 9:05 ")).toBe(d0 + 9 * H + 5 * 60_000);
  expect(atClock(day, "24:00")).toBe(d0 + 24 * H);
  expect(atClock(day, "9h05")).toBeNull();
  expect(atClock(day, "25:00")).toBeNull();
  expect(atClock(day, "10:60")).toBeNull();
  expect(atClock(day, "")).toBeNull();
});

test("snapAt maps a fraction of the axis to a whole minute", () => {
  const from = d0 + 8 * H, to = d0 + 10 * H;
  expect(snapAt(0, from, to)).toBe(from);
  expect(snapAt(1, from, to)).toBe(to);
  expect(hhmm(snapAt(0.5, from, to))).toBe("09:00");
  expect(hhmm(snapAt(0.25 + 10 / 7_200_000, from, to))).toBe("08:30");
  expect(snapAt(-1, from, to)).toBe(from);
  expect(snapAt(2, from, to)).toBe(to);
});

test("rangeError says what the server would refuse", () => {
  const now = d0 + 12 * H;
  expect(rangeError(d0 + 9 * H, d0 + 10 * H, now)).toBe("");
  expect(rangeError(null, d0 + 10 * H, now)).toBe("Times are HH:MM");
  expect(rangeError(d0 + 10 * H, d0 + 9 * H, now)).toBe("The range ends before it starts");
  expect(rangeError(d0 + 10 * H, d0 + 10 * H, now)).toBe("The range ends before it starts");
  expect(rangeError(d0 + 11 * H, d0 + 13 * H, now)).toBe("The range reaches into the future");
});

test("clockOf writes the end of the day as 24:00", () => {
  expect(clockOf(day, d0 + 9 * H + 5 * 60_000)).toBe("09:05");
  expect(clockOf(day, d0)).toBe("00:00");
  expect(clockOf(day, parseDate("2026-11-06").getTime())).toBe("24:00");
  expect(atClock(day, clockOf(day, parseDate("2026-11-06").getTime()))).toBe(parseDate("2026-11-06").getTime());
});
