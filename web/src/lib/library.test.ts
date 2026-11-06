import { test, expect } from "vitest";
import { seenText, isStale, rateText, matches, goalTarget } from "./library";

const DAY = 86_400_000;
const NOW = Date.UTC(2026, 10, 6, 12);

test("seenText", () => {
  expect(seenText(NOW - 3_600_000, NOW)).toBe("today");
  expect(seenText(NOW - 3 * DAY, NOW)).toBe("3 d ago");
  expect(seenText(NOW - 45 * DAY, NOW)).toBe("45 d ago");
  expect(seenText(0, NOW)).toBe("over 90 d");
});

test("isStale is over 30 days or unseen", () => {
  expect(isStale(NOW - 29 * DAY, NOW)).toBe(false);
  expect(isStale(NOW - 31 * DAY, NOW)).toBe(true);
  expect(isStale(0, NOW)).toBe(true);
});

test("rateText", () => {
  expect(rateText(45, "")).toBe("45/h");
  expect(rateText(45.5, "EUR")).toBe("45.5 EUR/h");
  expect(rateText(null, "EUR")).toBe("");
});

test("matches folds case over name and extras", () => {
  expect(matches("tock", ["Code"], "")).toBe(true);
  expect(matches("tock", ["Code"], "TO")).toBe(true);
  expect(matches("tock", ["Code"], "code")).toBe(true);
  expect(matches("tock", ["Code"], "atlas")).toBe(false);
});

test("goalTarget reads the goal without its label", () => {
  expect(goalTarget({ category: "Code", project: "", atMost: false, week: false, ms: 3 * 3_600_000 })).toBe("at least 3h00 a day");
  expect(goalTarget({ category: "Fun", project: "", atMost: true, week: true, ms: 5_400_000 })).toBe("at most 1h30 a week");
});
