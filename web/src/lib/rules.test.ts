import { test, expect } from "vitest";
import { labelOf, labelValue, parts, periods, statusText } from "./rules";

const base = { bundle: [] as string[], app: "", title: "", url: "", dir: "" };

test("parts lists the match fields in order", () => {
  expect(parts({ ...base, bundle: ["term"], title: "^tock" })).toEqual([
    { field: "Bundle", value: "term" },
    { field: "Title", value: "^tock" },
  ]);
  expect(parts({ ...base, url: "youtube\\.com", app: "Arc" }).map((p) => p.field)).toEqual(["App", "Site"]);
});

test("labelOf and labelValue", () => {
  expect(labelOf({ category: "Code", project: "tock" })).toBe("Code / tock");
  expect(labelOf({ category: "Code", project: "" })).toBe("Code");
  expect(labelOf({ category: "", project: "tock" })).toBe("/ tock");
  expect(labelValue({ category: "Code", project: "tock" })).toBe("Code/tock");
  expect(labelValue({ category: "", project: "tock" })).toBe("/tock");
});

test("statusText", () => {
  expect(statusText("unused")).toBe("never matched");
  expect(statusText("shadowed")).toBe("always beaten");
  expect(statusText("")).toBe("");
});

test("periods start with the month before", () => {
  const p = periods("2026-11-02");
  expect(p[0]).toMatchObject({ from: "2026-10-01", to: "2026-10-31" });
  expect(p[1]).toMatchObject({ from: "2026-11-01", to: "2026-11-30" });
  expect(p[2]).toEqual({ label: "This week", from: "2026-11-02", to: "2026-11-08" });
  expect(periods("2026-01-05")[0]).toMatchObject({ from: "2025-12-01", to: "2025-12-31" });
});
