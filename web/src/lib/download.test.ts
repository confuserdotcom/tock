import { test, expect } from "vitest";
import { fileName } from "./download";

test("fileName reads the attachment name", () => {
  expect(fileName('attachment; filename="tock-week-2026-11-02.html"', "x")).toBe("tock-week-2026-11-02.html");
  expect(fileName("attachment; filename=tock.csv", "x")).toBe("tock.csv");
  expect(fileName(null, "tock.csv")).toBe("tock.csv");
  expect(fileName('attachment; filename="../../etc/passwd"', "tock.csv")).toBe("passwd");
});
