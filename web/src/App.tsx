import { useEffect, useState } from "react";
import { HelpCircle, Search, Settings as Gear } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useApi, type Inbox as InboxT, type Status } from "@/lib/api";
import { holdsToday, hhmm, isoDate, stepDate, dur } from "@/lib/format";
import { DATED, go, useRoute, VIEWS, type View } from "@/lib/route";
import { cn } from "@/lib/utils";
import { GroupsContext, groupsOf } from "@/lib/groups";
import Today from "@/views/Today";
import Week from "@/views/Week";
import Trends from "@/views/Trends";
import Library from "@/views/Library";
import Top from "@/views/Top";
import Inbox from "@/views/Inbox";
import Settings from "@/views/Settings";
import Palette, { openPalette } from "@/components/Palette";
import { DateControl } from "@/components/DateControl";
import { openShortcuts, Shortcuts } from "@/components/Shortcuts";

// NATIVE: inside the tock window, which leaves room for the traffic lights.
const NATIVE = /tock-app/.test(navigator.userAgent);

const TITLES: Record<View, string> = { today: "Today", week: "Week", trends: "Trends", top: "Top", inbox: "Inbox", library: "Library", settings: "Settings" };

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
  const [n, setN] = useState(0);
  const status = useApi<Status>("/api/status", tick + n);
  const inbox = useApi<InboxT>("/api/inbox?days=7", tick);
  const unit = DATED[route.view];
  const step = (k: number) => unit && go({ date: stepDate(route.date, unit, k) });

  useEffect(() => {
    document.title = `tock · ${TITLES[route.view]}`;
  }, [route.view]);

  // 1-6 switch views, ",", settings, ← → step the date, t back to today;
  // outside text fields and dialogs.
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || (e.target as HTMLElement).closest("input,textarea,select,[role=dialog],[role=menu],[role=listbox]")) return;
      const k = Number(e.key);
      if (k >= 1 && k <= VIEWS.length) go({ view: VIEWS[k - 1] });
      else if (e.key === ",") go({ view: "settings" });
      else if (e.key === "ArrowLeft" && unit) step(-1);
      else if (e.key === "ArrowRight" && unit && !holdsToday(route.date, unit)) step(1);
      else if (e.key === "t" && unit) go({ date: isoDate(new Date()) });
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  });

  return (
    <GroupsContext.Provider value={groupsOf(status.data)}>
    <TooltipProvider delayDuration={150}>
      <div className="flex min-h-full flex-col">
        <header className="flex h-11 shrink-0 items-center gap-5 border-b px-6 [-webkit-app-region:drag]">
          <button
            onClick={() => go({ view: "today", date: isoDate(new Date()) })}
            className={cn("rounded-md font-semibold tracking-tight [-webkit-app-region:no-drag] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring", NATIVE && "ml-14")}
            title="Today"
          >
            tock
          </button>
          <nav className="flex gap-1 overflow-x-auto [-webkit-app-region:no-drag]" aria-label="Views">
            {VIEWS.map((v, i) => (
              <button
                key={v}
                onClick={() => go({ view: v })}
                aria-current={route.view === v ? "page" : undefined}
                title={`${TITLES[v]} (${i + 1})`}
                className={cn(
                  "rounded-md px-2.5 py-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
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
            {unit && <DateControl date={route.date} unit={unit} onDate={(d) => go({ date: d })} />}
            {unit && <span className="mx-2 h-4 w-px bg-border" aria-hidden />}
            <IconButton label="Search views and dates (⌘K)" onClick={openPalette}><Search /></IconButton>
            <IconButton label="Keys and gestures (?)" onClick={openShortcuts}><HelpCircle /></IconButton>
            <IconButton label="Settings (,)" onClick={() => go({ view: "settings" })} active={route.view === "settings"}><Gear /></IconButton>
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
          {route.view === "settings" && <Settings onChange={() => setN((x) => x + 1)} />}
        </main>
      </div>
      <Palette />
      <Shortcuts />
      <Toaster position="bottom-right" />
    </TooltipProvider>
    </GroupsContext.Provider>
  );
}

function IconButton({ label, onClick, active, children }: { label: string; onClick: () => void; active?: boolean; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          onClick={onClick}
          aria-label={label}
          aria-current={active ? "page" : undefined}
          className={cn(
            "grid h-7 w-7 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [&_svg]:size-4",
            active && "bg-secondary text-foreground",
          )}
        >
          {children}
        </button>
      </TooltipTrigger>
      <TooltipContent className="text-xs">{label}</TooltipContent>
    </Tooltip>
  );
}

function StatusStrip({ s }: { s: Status | null }) {
  const now = Date.now();
  const sess = s?.focus === false ? null : s?.session;
  const done = sess ? Math.min(1, (now - sess.startMS) / Math.max(1, sess.plannedMS - sess.startMS)) : 0;
  const link = "rounded-sm underline-offset-2 hover:text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";
  return (
    <div className="flex h-[34px] shrink-0 items-center gap-6 border-b px-6 text-xs text-muted-foreground">
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="flex cursor-default items-center gap-2" tabIndex={0}>
            <span className={cn("h-1.5 w-1.5 rounded-full", s?.recording ? "bg-k-deep" : "bg-k-distract")} />
            {s ? (s.recording ? "Recording" : s.paused ? "Not recording: config.toml has an error" : "trackd is not running") : "Connecting"}
          </span>
        </TooltipTrigger>
        <TooltipContent className="max-w-xs text-xs">
          {!s ? "Waiting for tock" : s.recording ? `trackd last wrote at ${hhmm(s.lastSeenMS)}` : s.paused ? s.paused : "Start it from the menu bar app, or run tock doctor"}
        </TooltipContent>
      </Tooltip>
      <button className={cn(link, s?.rulesError && "text-bad")} onClick={() => go({ view: "library", sub: "rules" })} title={s?.rulesError || "Open the rules"}>
        {s?.rulesError ? "Rules have an error" : "Rules OK"}
      </button>
      {s?.configError && (
        <button className={cn(link, "text-bad")} onClick={() => go({ view: "settings" })} title={s.configError}>Config has an error</button>
      )}
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
