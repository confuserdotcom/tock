import { Panel } from "@/components/Panel";
import { useApi, type Agents, type Budget } from "@/lib/api";
import { dur } from "@/lib/format";
import { cn } from "@/lib/utils";

/** AgentsAndBudgets is the day's agent ledger and the project budgets, each
 *  shown only when there is something in it. */
export function AgentsAndBudgets({ date, tick }: { date: string; tick: number }) {
  const { data: ag } = useApi<Agents>(`/api/agents?date=${date}`, tick);
  const { data: bs } = useApi<Budget[]>("/api/budgets", tick);
  const agents = ag && ag.rows.length > 0 ? ag : null;
  const budgets = bs && bs.length > 0 ? bs : null;
  if (!agents && !budgets) return null;
  return (
    <div id="agents-budgets" className={cn("grid gap-5", agents && budgets && "lg:grid-cols-2")}>
      {agents && <AgentsPanel ag={agents} />}
      {budgets && <BudgetsPanel bs={budgets} />}
    </div>
  );
}

function AgentsPanel({ ag }: { ag: Agents }) {
  const lev = ag.youMS > 0 ? ag.agentMS / ag.youMS : 0;
  return (
    <Panel label="Agents" aside={<span className="num">{dur(ag.agentMS)} worked{lev >= 1 ? ` · ${lev.toFixed(1)}× your time` : ""}</span>}>
      <table className="w-full text-sm">
        <thead>
          <tr className="lbl text-left">
            <th className="pb-2 font-medium">Project</th>
            <th className="pb-2 text-right font-medium">Agent</th>
            <th className="pb-2 text-right font-medium">You</th>
            <th className="pb-2 text-right font-medium">Prompts</th>
          </tr>
        </thead>
        <tbody className="num">
          {ag.rows.slice(0, 6).map((r) => (
            <tr key={r.project} className="border-t">
              <td className="max-w-0 truncate py-1.5 pr-3 font-sans" title={r.project}>{r.project}</td>
              <td className="py-1.5 text-right">{dur(r.agentMS)}</td>
              <td className="py-1.5 text-right text-muted-foreground">{dur(r.youMS)}</td>
              <td className="py-1.5 text-right text-muted-foreground">{r.prompts}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="mt-3 text-xs text-faint">
        From Claude Code's session logs: the time a session worked, and yours in its window.
        {ag.peak > 1 && ` Up to ${ag.peak} at once, ${dur(ag.wallMS)} of clock time.`}
      </p>
    </Panel>
  );
}

function BudgetsPanel({ bs }: { bs: Budget[] }) {
  return (
    <Panel label="Budgets">
      <ul className="space-y-3">
        {bs.map((b) => (
          <li key={b.project}>
            <div className="flex items-baseline gap-2 text-sm">
              <span className="min-w-0 flex-1 truncate">{b.project}</span>
              <span className="num">{dur(b.spentMS)} <span className="text-muted-foreground">/ {b.budget}</span></span>
              <span className={cn("num w-12 text-right", b.over && "text-[hsl(var(--k-distract))]")}>{b.pct}%</span>
            </div>
            <div className="mt-1 h-1.5 overflow-hidden rounded-[2px] bg-secondary">
              <div className="h-full rounded-[2px]" style={{ width: `${Math.min(b.pct, 100)}%`,
                background: b.over ? "hsl(var(--k-distract))" : b.pct >= 80 ? "hsl(var(--foreground) / 0.8)" : "hsl(var(--foreground) / 0.45)" }} />
            </div>
            <div className="num mt-1 text-xs text-muted-foreground">
              {b.over ? `over by ${dur(b.spentMS - b.budgetMS)}`
                : b.runsOutMS ? `${dur(b.perDayMS)} a day this week · runs out ${runsOut(b.runsOutMS)}`
                : "no time this week"}
              {b.since && ` · since ${b.since}`}
            </div>
          </li>
        ))}
      </ul>
    </Panel>
  );
}

/** runsOut is a day like "Fri 8 Jan", with the year when it is not this one. */
function runsOut(ms: number): string {
  const d = new Date(ms);
  const opts: Intl.DateTimeFormatOptions = { weekday: "short", day: "numeric", month: "short" };
  if (d.getFullYear() !== new Date().getFullYear()) opts.year = "numeric";
  return d.toLocaleDateString("en-GB", opts);
}
