import { expect, test } from "vitest";
import { arcs, hourAt } from "./dial";

const H = 3_600_000, M = 60_000;
const day = new Date(2026, 10, 13).getTime();
const row = (from: number, to: number, kind: string, idle = false) => ({ start: day + from, end: day + to, kind, idle });

test("runs of one kind with small gaps merge into one arc", () => {
  const a = arcs([row(9 * H, 9 * H + 20 * M, "deep"), row(9 * H + 21 * M, 10 * H, "deep"), row(10 * H, 11 * H, "shallow")], day);
  expect(a).toEqual([
    { from: 9 / 24, to: 10 / 24, kind: "deep", idle: false },
    { from: 10 / 24, to: 11 / 24, kind: "shallow", idle: false },
  ]);
});

test("time outside the day is cut off and slivers are dropped", () => {
  const a = arcs([row(-2 * H, 1 * H, "deep"), row(12 * H, 12 * H + 10_000, "browse"), row(23 * H, 26 * H, "neutral")], day);
  expect(a).toEqual([
    { from: 0, to: 1 / 24, kind: "deep", idle: false },
    { from: 23 / 24, to: 1, kind: "neutral", idle: false },
  ]);
});

test("away time stays apart from work of the same kind", () => {
  const a = arcs([row(9 * H, 10 * H, "", true), row(10 * H, 11 * H, "")], day);
  expect(a.map((x) => x.idle)).toEqual([true, false]);
});

test("hourAt reads the hour under a point of the dial, midnight at the top", () => {
  expect(hourAt(0, -1)).toBe(0);
  expect(hourAt(1, 0)).toBe(6);
  expect(hourAt(0, 1)).toBe(12);
  expect(hourAt(-1, 0)).toBe(18);
  expect(hourAt(-0.01, -1)).toBe(23);
});
