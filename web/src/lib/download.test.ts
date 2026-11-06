import { test, expect } from "vitest";
import { fileName } from "./download";

test("fileName reads the attachment name", () => {
  expect(fileName('attachment; filename="tock-week-2026-11-02.html"', "x")).toBe("tock-week-2026-11-02.html");
  expect(fileName("attachment; filename=tock.csv", "x")).toBe("tock.csv");
  expect(fileName(null, "tock.csv")).toBe("tock.csv");
  expect(fileName('attachment; filename="../../etc/passwd"', "tock.csv")).toBe("passwd");
});

test("fileName prefers the RFC 5987 name and never returns its prefix", () => {
  expect(fileName("attachment; filename*=UTF-8''tock%20week.csv", "x")).toBe("tock week.csv");
  expect(fileName(`attachment; filename="a.csv"; filename*=UTF-8''b.csv`, "x")).toBe("b.csv");
  expect(fileName("attachment; filename*=UTF-8''%E0%A4%A", "x.csv")).toBe("x.csv");
});
