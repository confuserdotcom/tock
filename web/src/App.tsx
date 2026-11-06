import { useEffect, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useApi, type Inbox as InboxT, type Status } from "@/lib/api";
import { addDays, dayTitle, isoDate, parseDate, dur } from "@/lib/format";
import { go, useRoute, VIEWS, type View } from "@/lib/route";
import { cn } from "@/lib/utils";
import Today from "@/views/Today";
import Week from "@/views/Week";
import Trends from "@/views/Trends";
import Library from "@/views/Library";
import Top from "@/views/Top";
import Inbox from "@/views/Inbox";
import Palette from "@/components/Palette";

// NATIVE: inside the tock window, which leaves room for the traffic lights.
const NATIVE = /tock-app/.test(navigator.userAgent);

const TITLES: Record<View, string> = { today: "Today", week: "Week", trends: "Trends", top: "Top", inbox: "Inbox", library: "Library" };

/** useTick changes once a minute, so live views refresh. */
function useTick(ms = 60_000) {
  const [t, setT] = useState(0);
  useEffect(() => {
    const id = setInterval(() => document.visibilityState === "visible" && setT((x) => x + 1), ms);
    return () => clearInterval(id);
  }, [ms]);
  return t;
}

export default function App() {
  const route = useRoute();
  const tick = useTick();
  const status = useApi<Status>("/api/status", tick);
  const inbox = useApi<InboxT>("/api/inbox?days=7", tick);
  const date = parseDate(route.date);
  const isToday = route.date === isoDate(new Date());
  const step = (n: number) => go({ date: isoDate(addDays(date, n)) });

  useEffect(() => {
    document.title = `tock · ${TITLES[route.view]}`;
  }, [route.view]);

  // 1-5 switch views, ← → move between days, outside text fields and dialogs.
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || (e.target as HTMLElement).closest("input,textarea,[role=dialog]")) return;
      const n = Number(e.key);
      if (n >= 1 && n <= VIEWS.length) go({ view: VIEWS[n - 1] });
      else if (e.key === "ArrowLeft" && route.view !== "inbox" && route.view !== "library") step(-1);
      else if (e.key === "ArrowRight" && route.view !== "inbox" && route.view !== "library" && !isToday) step(1);
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  });

  return (
    <TooltipProvider delayDuration={150}>
      <div className="flex min-h-full flex-col">
        <header className="flex h-11 shrink-0 items-center gap-7 border-b px-6 [-webkit-app-region:drag]">
          <span className={cn("font-semibold tracking-tight", NATIVE && "pl-14")}>tock</span>
          <nav className="flex gap-1 [-webkit-app-region:no-drag]" aria-label="Views">
            {VIEWS.map((v) => (
              <button
                key={v}
                onClick={() => go({ view: v })}
                aria-current={route.view === v ? "page" : undefined}
                className={cn(
                  "rounded-md px-2.5 py-1 text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                  route.view === v && "bg-secondary text-foreground",
                )}
              >
                {TITLES[v]}
                {v === "inbox" && inbox.data && inbox.data.items.length > 0 && (
                  <Badge variant="secondary" className="ml-1.5 h-4 px-1 text-[10px]">{inbox.data.items.length}</Badge>
                )}
              </button>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-1 [-webkit-app-region:no-drag]">
{route.view !== "library" && (
              <>
            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => step(-1)} aria-label="Previous day">
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <button className="min-w-24 text-center font-medium" onClick={() => go({ date: isoDate(new Date()) })} title="Go to today">
              {isToday ? "Today" : dayTitle(date)}
            </button>
            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => step(1)} disabled={isToday} aria-label="Next day">
              <ChevronRight className="h-4 w-4" />
            </Button>
              </>
            )}
            <kbd className="ml-3 hidden rounded border px-1.5 text-[11px] text-faint md:inline">⌘K</kbd>
          </div>
        </header>
        <StatusStrip s={status.data} />
        <main className="mx-auto w-full max-w-[1400px] flex-1 px-6 py-5">
          {route.view === "today" && <Today date={route.date} tick={tick} />}
          {route.view === "week" && <Week date={route.date} tick={tick} />}
          {route.view === "trends" && <Trends date={route.date} />}
          {route.view === "top" && <Top tick={tick} />}
          {route.view === "inbox" && <Inbox onChange={inbox.reload} />}
          {route.view === "library" && <Library tick={tick} sub={route.sub} />}
        </main>
      </div>
      <Palette />
      <Toaster position="bottom-right" />
    </TooltipProvider>
  );
}

function StatusStrip({ s }: { s: Status | null }) {
  const now = Date.now();
  const sess = s?.session;
  const done = sess ? Math.min(1, (now - sess.startMS) / Math.max(1, sess.plannedMS - sess.startMS)) : 0;
  return (
    <div className="flex h-[34px] shrink-0 items-center gap-6 border-b px-6 text-xs text-muted-foreground">
      <span className="flex items-center gap-2">
        <span className={cn("h-1.5 w-1.5 rounded-full", s?.recording ? "bg-k-deep" : "bg-k-distract")} />
        {s ? (s.recording ? "Recording" : "trackd is not running") : "Connecting"}
      </span>
      <span className={cn(s?.rulesError && "text-bad")}>{s?.rulesError ? "Rules have an error" : "Rules OK"}</span>
      {s?.configError && <span className="text-bad">Config has an error</span>}
      {sess && (
        <span className="ml-auto flex items-center gap-2 text-foreground">
          {sess.goal || "Focus session"}
          <span className="inline-block h-1 w-24 overflow-hidden rounded bg-secondary">
            <i className="block h-full bg-foreground" style={{ width: `${done * 100}%` }} />
          </span>
          <span className="num text-muted-foreground">
            {dur(now - sess.startMS)} / {dur(sess.plannedMS - sess.startMS)}
          </span>
        </span>
      )}
    </div>
  );
}
