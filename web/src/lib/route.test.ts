import { test, expect, vi, afterEach } from "vitest";

afterEach(() => vi.unstubAllGlobals());

async function parseAt(hash: string) {
  vi.resetModules();
  vi.stubGlobal("location", { hash, search: "" });
  const { parse } = await import("./route");
  return parse();
}

test("library sections ride in the second segment", async () => {
  expect(await parseAt("#library/goals")).toMatchObject({ view: "library", sub: "goals" });
  expect(await parseAt("#library")).toMatchObject({ view: "library", sub: "projects" });
  expect(await parseAt("#library/nonsense")).toMatchObject({ view: "library", sub: "projects" });
  expect(await parseAt("#week/2026-11-02")).toMatchObject({ view: "week", date: "2026-11-02" });
});

test("settings is a view without a date, and dates stay on dated views", async () => {
  expect(await parseAt("#settings")).toMatchObject({ view: "settings" });
  expect(await parseAt("#trends/2026-10-01")).toMatchObject({ view: "trends", date: "2026-10-01" });
  const top = await parseAt("#top/2026-10-01");
  expect(top.view).toBe("top");
  expect(top.date).not.toBe("2026-10-01");
});
