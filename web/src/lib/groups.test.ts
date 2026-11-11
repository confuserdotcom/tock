import { describe, expect, it } from "vitest";
import { groupsOf, weekExports } from "./groups";

describe("groups", () => {
  it("reads status, missing fields on", () => {
    expect(groupsOf(null)).toEqual({ focus: true, billing: true });
    expect(groupsOf({ focus: false, billing: true })).toEqual({ focus: false, billing: true });
  });

  it("Week Export with billing on and off", () => {
    const on = weekExports(true);
    expect(on.invoice).toBe(true);
    expect(on.items.find((e) => e.q === "by=project")?.sub).toMatch(/rate/);
    const off = weekExports(false);
    expect(off.invoice).toBe(false);
    expect(off.items.map((e) => e.sub).join(" ")).not.toMatch(/rate|amount/);
    expect(off.items).toHaveLength(on.items.length);
  });
});
