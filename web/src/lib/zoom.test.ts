import { expect, test } from "vitest";
import { clampWin, fit, isFull, MIN_SPAN, pan, ticks, zoomAt } from "./zoom";

const H = 3_600_000;
const day = new Date(2026, 9, 10).getTime();
const b = { from: day + 8 * H, to: day + 18 * H };

test("zooming keeps the point under the pointer in place", () => {
  const w = zoomAt(b, 0.5, 0.5, b);
  expect(w).toEqual({ from: day + 10.5 * H, to: day + 15.5 * H });
  const left = zoomAt(b, 0, 0.5, b);
  expect(left.from).toBe(b.from);
});

test("the window stays inside the axis and at least a quarter hour wide", () => {
  expect(zoomAt(b, 0.5, 2, b)).toEqual(b);
  const tiny = zoomAt(b, 0.5, 0.0001, b);
  expect(tiny.to - tiny.from).toBe(MIN_SPAN);
  const w = { from: day + 9 * H, to: day + 10 * H };
  expect(pan(w, -5, b)).toEqual({ from: b.from, to: b.from + H });
  expect(pan(w, 50, b)).toEqual({ from: b.to - H, to: b.to });
  expect(clampWin({ from: b.from, to: b.from + 1000 }, b).to - b.from).toBe(MIN_SPAN);
});

test("fit leaves room around a range and says when it is the whole day", () => {
  const w = fit(day + 12 * H, day + 13 * H, b);
  expect(w.from).toBeLessThan(day + 12 * H);
  expect(w.to).toBeGreaterThan(day + 13 * H);
  expect(isFull(w, b)).toBe(false);
  expect(isFull(b, b)).toBe(true);
});

test("ticks land on round local times and thin out with the span", () => {
  const hourly = ticks(b);
  expect(hourly.length).toBe(11);
  expect(new Date(hourly[0]).getHours()).toBe(8);
  expect(new Date(hourly[0]).getMinutes()).toBe(0);
  const close = ticks({ from: day + 9 * H + 7 * 60_000, to: day + 9 * H + 40 * 60_000 });
  expect(close.map((t) => new Date(t).getMinutes())).toEqual([10, 15, 20, 25, 30, 35, 40]);
});
