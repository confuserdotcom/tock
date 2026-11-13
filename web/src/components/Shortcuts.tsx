import { useEffect, useState } from "react";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";

const GROUPS: [string, [string, string][]][] = [
  ["Anywhere", [
    ["1 – 6", "Today, Week, Trends, Top, Inbox, Library"],
    [",", "Settings"],
    ["← →", "Previous or next day, week or month"],
    ["t", "Back to today"],
    ["f", "Name and start a focus session"],
    ["⌘K", "Go to a view or a date"],
    ["⌘+  ⌘−  ⌘0", "Zoom the page"],
    ["?", "This list"],
  ]],
  ["Timeline", [
    ["Click a bar", "Label it or delete it"],
    ["Drag across", "Label, add or delete a range"],
    ["⌘ scroll, pinch", "Zoom around the pointer"],
    ["Shift-drag", "Zoom to a range"],
    ["Double click", "Zoom in there"],
    ["+  −  0", "Zoom in, out, whole day (timeline focused)"],
    ["Sideways scroll", "Move along the day when zoomed in"],
  ]],
  ["Inbox", [
    ["j k", "Move down, up"],
    ["Return", "Accept the guess and learn a rule"],
    ["l", "Pick a label"],
    ["i", "Ignore"],
    ["u", "Undo"],
  ]],
];

/** openShortcuts shows the list from anywhere, as ? does. */
export function openShortcuts() {
  window.dispatchEvent(new Event("tock:shortcuts"));
}

/** Shortcuts: the keyboard and pointer moves, on ?. */
export function Shortcuts() {
  const [open, setOpen] = useState(false);
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if (e.key !== "?" || e.metaKey || e.ctrlKey || (e.target as HTMLElement).closest("input,textarea,[role=dialog]")) return;
      e.preventDefault();
      setOpen((o) => !o);
    };
    const show = () => setOpen(true);
    window.addEventListener("keydown", on);
    window.addEventListener("tock:shortcuts", show);
    return () => (window.removeEventListener("keydown", on), window.removeEventListener("tock:shortcuts", show));
  }, []);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Keys and gestures</DialogTitle>
          <DialogDescription>Everything here also works with the mouse.</DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] space-y-4 overflow-y-auto">
          {GROUPS.map(([g, rows]) => (
            <section key={g}>
              <h3 className="mb-1.5 text-xs font-medium text-muted-foreground">{g}</h3>
              <dl className="grid grid-cols-[9rem_1fr] gap-x-4 gap-y-1">
                {rows.map(([k, v]) => (
                  <div key={k} className="contents">
                    <dt><kbd className="rounded border bg-secondary px-1.5 py-0.5 font-sans text-[11px]">{k}</kbd></dt>
                    <dd>{v}</dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
