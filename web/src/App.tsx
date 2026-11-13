import { useEffect, useState } from "react";
import { BarChart3, CalendarDays, HelpCircle, Inbox as InboxIcon, Library as LibraryIcon, ListOrdered, Search, Settings as Gear, Sun } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useApi, type Inbox as InboxT, type Status } from "@/lib/api";
import { holdsToday, hhmm, isoDate, stepDate, dur } from "@/lib/format";
import { DATED, go, useRoute, VIEWS, type View } from "@/lib/route";
import { cn } from "@/lib/utils";
import { GroupsContext, groupsOf } from "@/lib/groups";
import { LiveContext } from "@/lib/live";
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
import { FocusDock, focusField } from "@/components/FocusControl";

// NATIVE: inside the tock window, which leaves room for the traffic lights.
const NATIVE = /tock-app/.test(navigator.userAgent);

const TITLES: Record<View, string> = { today: "Today", week: "Week", trends: "Trends", top: "Top", inbox: "Inbox", library: "Library", settings: "Settings" };
const ICONS: Record<View, typeof Sun> = { today: Sun, week: CalendarDays, trends: BarChart3, top: ListOrdered, inbox: InboxIcon, library: LibraryIcon, settings: Gear };

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
  const reload = () => setN((x) => x + 1);
  const inboxN = inbox.data?.items.length ?? 0;

  useEffect(() => {
    document.title = `tock · ${TITLES[route.view]}`;
  }, [route.view]);

  // 1-6 switch views, ",", settings, ← → step the date, t back to today,
  // f to the focus field; outside text fields and dialogs.
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || (e.target as HTMLElement).closest("input,textarea,select,[role=dialog],[role=menu],[role=listbox]")) return;
      const k = Number(e.key);
      if (k >= 1 && k <= VIEWS.length) go({ view: VIEWS[k - 1] });
      else if (e.key === ",") go({ view: "settings" });
      else if (e.key === "ArrowLeft" && unit) step(-1);
      else if (e.key === "ArrowRight" && unit && !holdsToday(route.date, unit)) step(1);
      else if (e.key === "t" && unit) go({ date: isoDate(new Date()) });
      else if (e.key === "f") {
        if (route.view !== "today" || !holdsToday(route.date, "day")) go({ view: "today", date: isoDate(new Date()) });
        setTimeout(focusField, 60);
      } else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", on);
    return () => window.removeEventListener("keydown", on);
  });

  const live = { status: status.data, reload, inboxMS: inbox.data?.totalMS ?? 0, inboxN };

  return (
    <LiveContext.Provider value={live}>
    <GroupsContext.Provider value={groupsOf(status.data)}>
    <TooltipProvider delayDuration={150}>
      <div className="flex min-h-full">
        <aside className="sticky top-0 hidden h-screen w-[228px] shrink-0 flex-col border-r bg-card/50 md:flex">
          <div className={cn("flex h-14 shrink-0 items-center px-5 [-webkit-app-region:drag]", NATIVE && "h-[76px] items-end pb-3")}>
            <button onClick={() => go({ view: "today", date: isoDate(new Date()) })}
              className={cn("flex items-center gap-2 rounded-md text-[15px] font-semibold tracking-tight [-webkit-app-region:no-drag] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring")} title="Today">
              <Mark /> tock
            </button>
          </div>
          <nav className="mt-2 flex flex-col gap-0.5 px-3" aria-label="Views">
            {VIEWS.map((v, i) => <NavItem key={v} v={v} k={String(i + 1)} active={route.view === v} count={v === "inbox" ? inboxN : 0} />)}
          </nav>
          <div className="mt-auto space-y-3 p-3">
            <FocusDock />
            <Health s={status.data} />
            <div className="flex items-center gap-0.5 border-t px-1 pt-2">
              <NavItem v="settings" k="," active={route.view === "settings"} count={0} />
              <span className="ml-auto" />
              <IconButton label="Keys and gestures (?)" onClick={openShortcuts}><HelpCircle /></IconButton>
            </div>
          </div>
        </aside>

        <div className="flex min-w-0 flex-1 flex-col">
          <header className={cn("sticky top-0 z-20 flex h-14 shrink-0 items-center gap-4 border-b bg-background/85 px-6 backdrop-blur [-webkit-app-region:drag]")}>
            <h1 className={cn("text-[20px] font-semibold tracking-[-0.01em]", NATIVE && "ml-16 md:ml-0")}>{TITLES[route.view]}</h1>
            <div className="ml-auto flex items-center gap-1 [-webkit-app-region:no-drag]">
              {unit && <DateControl date={route.date} unit={unit} onDate={(d) => go({ date: d })} />}
              {unit && <span className="mx-2 h-4 w-px bg-border" aria-hidden />}
              <button onClick={openPalette}
                className="hidden h-8 items-center gap-2 rounded-md border bg-card px-2.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:flex">
                <Search className="h-3.5 w-3.5" /> Go to <kbd className="ml-2 rounded border bg-secondary px-1 font-sans text-[10px]">⌘K</kbd>
              </button>
              <span className="sm:hidden"><IconButton label="Search views and dates (⌘K)" onClick={openPalette}><Search /></IconButton></span>
            </div>
          </header>
          <nav className="flex gap-1 overflow-x-auto border-b px-4 py-2 md:hidden" aria-label="Views">
            {[...VIEWS, "settings" as const].map((v) => {
              const I = ICONS[v];
              return (
                <button key={v} onClick={() => go({ view: v })} aria-current={route.view === v ? "page" : undefined} aria-label={TITLES[v]}
                  className={cn("grid h-11 min-w-11 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent", route.view === v && "bg-secondary text-foreground")}>
                  <I className="h-4 w-4" />
                </button>
              );
            })}
          </nav>
          <main key={route.view} className="mx-auto w-full max-w-[1400px] flex-1 px-6 py-6 animate-in fade-in-0 slide-in-from-bottom-1 duration-200 motion-reduce:animate-none">
            {route.view === "today" && <Today date={route.date} tick={tick} />}
            {route.view === "week" && <Week date={route.date} tick={tick} />}
            {route.view === "trends" && <Trends date={route.date} />}
            {route.view === "top" && <Top tick={tick} />}
            {route.view === "inbox" && <Inbox onChange={inbox.reload} />}
            {route.view === "library" && <Library tick={tick} sub={route.sub} />}
            {route.view === "settings" && <Settings onChange={reload} />}
          </main>
        </div>
      </div>
      <Palette />
      <Shortcuts />
      <Toaster position="bottom-right" />
    </TooltipProvider>
    </GroupsContext.Provider>
    </LiveContext.Provider>
  );
}

/** Mark is tock's mark: a dial with one hand. */
function Mark() {
  return (
    <svg viewBox="0 0 20 20" className="h-5 w-5" aria-hidden>
      <circle cx="10" cy="10" r="8" fill="none" stroke="currentColor" strokeOpacity="0.35" strokeWidth="2" />
      <path d="M10 2a8 8 0 0 1 8 8" fill="none" stroke="hsl(var(--k-deep))" strokeWidth="2" strokeLinecap="round" />
      <path d="M10 10V5.5" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
    </svg>
  );
}

function NavItem({ v, k, active, count }: { v: View; k: string; active: boolean; count: number }) {
  const I = ICONS[v];
  return (
    <button onClick={() => go({ view: v })} aria-current={active ? "page" : undefined} title={`${TITLES[v]} (${k})`}
      className={cn(
        "group flex h-9 flex-1 items-center gap-3 rounded-md px-2.5 text-[13px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        active && "bg-secondary font-medium text-foreground",
      )}>
      <I className={cn("h-4 w-4 shrink-0", active ? "text-foreground" : "text-faint group-hover:text-muted-foreground")} />
      <span className="flex-1 text-left">{TITLES[v]}</span>
      {count > 0 && <span className="num rounded-full bg-foreground/10 px-1.5 text-[11px] font-medium text-foreground">{count}</span>}
      <kbd className="hidden font-sans text-[10px] text-faint opacity-0 transition-opacity group-hover:opacity-100 lg:block">{k}</kbd>
    </button>
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
            "grid h-8 w-8 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [&_svg]:size-4",
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

/** Health is whether tock is recording and its rules and config load. */
function Health({ s }: { s: Status | null }) {
  const link = "rounded-sm text-left underline-offset-2 hover:text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";
  const rec = s?.recording;
  return (
    <div className="space-y-1.5 px-1 text-xs text-muted-foreground">
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="flex cursor-default items-center gap-2" tabIndex={0}>
            <span className="relative flex h-2 w-2">
              {rec && <span className="absolute inset-0 animate-ping rounded-full bg-k-deep opacity-50 [animation-duration:2.4s] motion-reduce:hidden" />}
              <span className={cn("relative h-2 w-2 rounded-full", rec ? "bg-k-deep" : s ? "bg-k-distract" : "bg-faint")} />
            </span>
            {s ? (rec ? "Recording" : s.paused ? "Not recording: config error" : "trackd is not running") : "Connecting"}
          </span>
        </TooltipTrigger>
        <TooltipContent side="right" className="max-w-xs text-xs">
          {!s ? "Waiting for tock" : rec ? `trackd last wrote at ${hhmm(s.lastSeenMS)} · ${dur(Date.now() - s.lastSeenMS)} ago` : s.paused ? s.paused : "Start it from the menu bar app, or run tock doctor"}
        </TooltipContent>
      </Tooltip>
      {s?.rulesError && <button className={cn(link, "block text-bad")} onClick={() => go({ view: "library", sub: "rules" })} title={s.rulesError}>Rules have an error</button>}
      {s?.configError && <button className={cn(link, "block text-bad")} onClick={() => go({ view: "settings" })} title={s.configError}>Config has an error</button>}
    </div>
  );
}
