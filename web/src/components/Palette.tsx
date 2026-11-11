import { useEffect, useState } from "react";
import { CommandDialog, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command";
import { addDays, isoDate } from "@/lib/format";
import { go, VIEWS } from "@/lib/route";
import { setTheme, type Theme } from "@/lib/theme";
import { openShortcuts } from "@/components/Shortcuts";

const NAMES: Record<string, string> = { today: "Today", week: "Week", trends: "Trends", top: "Top", inbox: "Inbox", library: "Library" };

/** openPalette opens ⌘K from a button. */
export function openPalette() {
  window.dispatchEvent(new Event("tock:palette"));
}

/** Palette: ⌘K to jump to a view or a date, or switch the theme. */
export default function Palette() {
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if (e.key === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((o) => !o);
      }
    };
    const show = () => setOpen(true);
    window.addEventListener("keydown", on);
    window.addEventListener("tock:palette", show);
    return () => (window.removeEventListener("keydown", on), window.removeEventListener("tock:palette", show));
  }, []);
  const run = (f: () => void) => () => (f(), setOpen(false), setQ(""));
  const typed = /^\d{4}-\d{2}-\d{2}$/.test(q.trim()) ? q.trim() : null;
  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <CommandInput placeholder="Go to a view, a date (YYYY-MM-DD), or run a command" value={q} onValueChange={setQ} />
      <CommandList>
        <CommandEmpty>Nothing matches.</CommandEmpty>
        {typed && (
          <CommandGroup heading="Date">
            <CommandItem value={typed} onSelect={run(() => go({ view: "today", date: typed }))}>Open {typed}</CommandItem>
          </CommandGroup>
        )}
        <CommandGroup heading="Views">
          {VIEWS.map((v) => (
            <CommandItem key={v} value={NAMES[v]} onSelect={run(() => go({ view: v }))}>{NAMES[v]}</CommandItem>
          ))}
        </CommandGroup>
        <CommandGroup heading="Days">
          <CommandItem value="Go to today" onSelect={run(() => go({ date: isoDate(new Date()) }))}>Go to today</CommandItem>
          <CommandItem value="Go to yesterday" onSelect={run(() => go({ view: "today", date: isoDate(addDays(new Date(), -1)) }))}>Go to yesterday</CommandItem>
        </CommandGroup>
        <CommandGroup heading="Settings">
          <CommandItem value="Settings features notifications privacy" onSelect={run(() => go({ view: "settings" }))}>Open settings</CommandItem>
          {(["dark", "light", "system"] as Theme[]).map((t) => (
            <CommandItem key={t} value={`Theme ${t}`} onSelect={run(() => setTheme(t))}>Theme: {t === "system" ? "follow the system" : t}</CommandItem>
          ))}
          <CommandItem value="Keyboard shortcuts keys help" onSelect={run(openShortcuts)}>Keys and gestures</CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}
