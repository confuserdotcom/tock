import { expect, test } from "vitest";
import { moves } from "./moves";

const M = 60_000;
const goal = (o: Partial<Parameters<typeof moves>[0]["goals"][number]>) => ({
  name: "Writing", text: "", atMost: false, week: false, ms: 120 * M, doneMS: 0, state: "", streak: 0, best: 0, ...o,
});

test("unlabelled time in the inbox comes before goals still to reach", () => {
  const m = moves({ inboxMS: 134 * M, goals: [goal({ doneMS: 30 * M })], today: true });
  expect(m[0]).toEqual({ id: "inbox", text: "Label 2h14 waiting in the Inbox", to: "inbox", tone: "plain" });
  expect(m[1]).toMatchObject({ id: "goal:Writing", text: "1h30 more Writing reaches today's goal", tone: "plain" });
});

test("a met goal and a spent limit say so, a near limit warns", () => {
  const m = moves({
    inboxMS: 0, today: true,
    goals: [goal({ doneMS: 130 * M }), goal({ name: "Social", atMost: true, ms: 30 * M, doneMS: 40 * M }), goal({ name: "Mail", atMost: true, ms: 60 * M, doneMS: 50 * M })],
  });
  expect(m.map((x) => [x.text, x.tone])).toEqual([
    ["Social is 10m past its limit", "bad"],
    ["10m left of the Mail limit", "warn"],
    ["Writing goal met", "good"],
  ]);
});

test("a goal goes by its category's name, not its rule", () => {
  expect(moves({ inboxMS: 0, today: true, goals: [goal({ name: "Fun", text: "Fun ≤45m/day", atMost: true, ms: 45 * M, doneMS: 60 * M })] })[0].text)
    .toBe("Fun is 15m past its limit");
});

test("week goals, small inboxes and past days add nothing to do", () => {
  expect(moves({ inboxMS: 2 * M, goals: [goal({ week: true, days: 5, held: 2 })], today: true })).toEqual([]);
  expect(moves({ inboxMS: 90 * M, goals: [goal({})], today: false })).toEqual([]);
});
