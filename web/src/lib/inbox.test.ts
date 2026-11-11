import { test, expect } from "vitest";
import { guessText } from "./inbox";

test("guessText says the label, the reason and a split vote", () => {
  expect(guessText({ label: "Study", reason: "same site", share: 0.8 })).toBe("Study: same site, 80 % of the evidence");
  expect(guessText({ label: "Code", reason: "same app", share: 1 })).toBe("Code: same app");
  expect(guessText({ label: "Code" })).toBe("");
  expect(guessText(undefined)).toBe("");
});
