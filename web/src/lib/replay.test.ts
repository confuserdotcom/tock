import { describe, expect, it } from "vitest";
import { frame, ordered } from "./replay";
import type { Row } from "./api";

const m = 60_000;
const row = (start: number, end: number, app: string, kind: string, idle = false): Row => ({
  ids: [start], start: start * m, end: end * m, key: app, app, text: app + " title",
  category: kind ? "Cat" : "", kind, project: "", source: "", idle, open: false,
});

const rows = [
  row(0, 30, "Code", "deep"),
  row(30, 35, "Slack", "shallow"),
  row(35, 50, "Code", "deep"),
  row(50, 70, "Idle", "", true),
  row(70, 80, "YouTube", "distract"),
];

describe("frame", () => {
  it("finds the row at the playhead and tallies up to it", () => {
    const f = frame(rows, 40 * m, 3);
    expect(f.now?.app).toBe("Code");
    expect(f.workedMS).toBe(40 * m);
    expect(f.deepMS).toBe(35 * m);
    expect(f.switches).toBe(2);
    expect(f.longestMS).toBe(30 * m);
    expect(f.trail.map((r) => r.app)).toEqual(["Code", "Slack"]);
  });
  it("counts away time apart and leaves it off the switches", () => {
    const f = frame(rows, 75 * m, 10);
    expect(f.now?.app).toBe("YouTube");
    expect(f.awayMS).toBe(20 * m);
    expect(f.workedMS).toBe(55 * m);
    expect(f.distractMS).toBe(5 * m);
    expect(f.switches).toBe(3);
    expect(f.trail.map((r) => r.app)).toEqual(["Code", "Slack", "Code"]);
  });
  it("keeps blips out of the trail and joins an app's rows", () => {
    const blip = (s: number, app: string): Row => ({ ...row(0, 0, app, "deep"), start: s * m, end: s * m + 20_000 });
    const f = frame([row(0, 5, "A", "deep"), blip(5, "B"), row(6, 9, "A", "deep"), row(9, 12, "C", "deep"), row(12, 20, "D", "deep")], 15 * m, 5);
    expect(f.trail.map((r) => [r.app, r.start / m, r.end / m])).toEqual([["A", 0, 9], ["C", 9, 12]]);
  });
  it("is empty before the first row and in gaps", () => {
    expect(frame(rows, -m, 3).now).toBeNull();
    expect(frame([row(0, 10, "A", "deep"), row(20, 30, "B", "deep")], 15 * m, 3).now).toBeNull();
  });
});

describe("ordered", () => {
  it("sorts by start only when it has to", () => {
    expect(ordered(rows)).toBe(rows);
    const shuffled = [rows[2], rows[0], rows[1]];
    expect(ordered(shuffled).map((r) => r.start)).toEqual([0, 30 * m, 35 * m]);
    expect(shuffled[0]).toBe(rows[2]);
  });
});
