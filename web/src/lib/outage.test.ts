import { test, expect } from "vitest";
import { onDay } from "./outage";

const o = (from: number, to: number, kind = "crash") => ({ from, to, kind, why: "trackd stopped unexpectedly" });

test("onDay clips to the day and sums", () => {
  const got = onDay([o(0, 50), o(80, 150), o(200, 300)], 100, 200);
  expect(got.items).toEqual([o(100, 150)]);
  expect(got.ms).toBe(50);
});
test("onDay with none", () => {
  expect(onDay(undefined, 0, 10)).toEqual({ items: [], ms: 0 });
  expect(onDay([], 0, 10)).toEqual({ items: [], ms: 0 });
});
