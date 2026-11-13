import { expect, test } from "vitest";
import { left, presetName } from "./live";

test("countdowns read minutes and seconds, hours when there are some", () => {
  expect(left(24 * 60_000 + 5_000)).toBe("24:05");
  expect(left(62 * 60_000 + 5_000)).toBe("1:02:05");
  expect(left(-3)).toBe("0:00");
});

test("preset chips read 25m, 1h, 1h30", () => {
  expect([25, 60, 90].map(presetName)).toEqual(["25m", "1h", "1h30"]);
});
