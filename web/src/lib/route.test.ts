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
