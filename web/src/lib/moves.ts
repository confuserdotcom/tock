// Next moves: the few things worth doing about today, from the day's goals
// and the inbox. Pure, so the order is tested.
import type { Goal } from "./api";
import { dur } from "./format";

export type Move = { id: string; text: string; to?: "inbox" | "goals"; tone: "plain" | "good" | "warn" | "bad" };

const MIN_INBOX = 5 * 60_000; // less waiting in the inbox is not worth a move
const RANK: Record<string, number> = { bad: 0, warn: 1, inbox: 2, plain: 3, good: 4 };

/** moves lists what to do about today; a past day has none. */
export function moves({ inboxMS, goals, today }: { inboxMS: number; goals: Goal[]; today: boolean }): Move[] {
  if (!today) return [];
  const out: (Move & { rank: number })[] = [];
  if (inboxMS >= MIN_INBOX) out.push({ id: "inbox", text: `Label ${dur(inboxMS)} waiting in the Inbox`, to: "inbox", tone: "plain", rank: RANK.inbox });
  for (const g of goals) {
    if (g.week || g.days) continue; // a week's goal is held by days, not by today alone
    const name = g.text || g.name, left = g.ms - g.doneMS, id = `goal:${g.name}`;
    let m: Move;
    if (g.atMost) {
      if (left < 0) m = { id, text: `${name} is ${dur(-left)} past its limit`, to: "goals", tone: "bad" };
      else if (left <= g.ms / 4) m = { id, text: `${dur(left)} left of the ${name} limit`, to: "goals", tone: "warn" };
      else continue;
    } else if (left <= 0) m = { id, text: `${name} goal met`, to: "goals", tone: "good" };
    else m = { id, text: `${dur(left)} more ${name} reaches today's goal`, to: "goals", tone: "plain" };
    out.push({ ...m, rank: RANK[m.tone] });
  }
  return out.sort((a, b) => a.rank - b.rank).map(({ rank: _, ...m }) => m);
}
