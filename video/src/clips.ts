// What each clip says and where the camera goes. Shots aim at the boxes the
// capture logged (capture/scenes.mjs marks), or at boxes in page pixels
// (1440x900) for things the cursor doesn't touch.
import type { ClipDef } from "./Clip";
import { C } from "./theme";

export const CLIPS: ClipDef[] = [
  {
    id: "today",
    footage: "today",
    kicker: "Today",
    title: "Your day, without a timer.",
    shots: [
      { at: 0, frame: "rest", caption: "Every app and site, on one timeline.", sub: "trackd records it. You never press start.", dot: C.deep },
      { at: "bar-code", frame: "timeline", zoom: 1.4, caption: "One row per category.", sub: "Your rules label it, past days included.", dot: C.deep },
      { at: "score", frame: "score", caption: "A focus score out of 100.", sub: "Measured against your usual Thursday.", dot: C.ink },
      { at: "donut", frame: "donut", caption: "Deep, shallow, browsing, distracting.", sub: "Colour only ever means a kind of time.", dot: C.shallow },
      { at: 11.1, frame: "rest" },
      { at: "goals", frame: "goals", caption: "Goals per day or per week.", sub: "At least three hours of code. At most half an hour of feeds.", dot: C.distract },
    ],
  },
  {
    id: "edit",
    footage: "edit",
    kicker: "Fix the record",
    title: "Click a bar. Label it.",
    shots: [
      { at: "bar-unlabelled", frame: "timeline", zoom: 1.45, caption: "Twelve minutes nobody labelled.", sub: "Click the bar.", dot: C.neutral },
      { at: "sheet", frame: { x: 1040, y: 0, width: 400, height: 380 }, zoom: 1.9, caption: "Type a category, press return.", sub: "Or a project: Design / Northwind.", dot: C.browse },
      { at: "toast", frame: "toast", zoom: 1.5, caption: "Every change can be undone.", sub: "From the toast, for as long as it shows.", dot: C.ink },
      { at: "range", frame: "range", zoom: 1.35, caption: "Or drag across the lanes.", sub: "Label a range, add time tock missed, or delete it.", dot: C.shallow },
    ],
  },
  {
    id: "inbox",
    footage: "inbox",
    kicker: "Inbox",
    title: "Label each thing once.",
    shots: [
      { at: 0, frame: { x: 44, y: 60, width: 1352, height: 280 }, zoom: 1.45, caption: "Time no rule covers waits here.", sub: "Grouped by app, site or title.", dot: C.neutral },
      { at: "sheet", frame: { x: 1040, y: 0, width: 400, height: 380 }, zoom: 1.9, caption: "Label it, and tock writes the rule.", sub: "Next time it files the same thing by itself.", dot: C.browse },
      { at: "toast", frame: { x: 44, y: 60, width: 1352, height: 280 }, zoom: 1.45, caption: "The inbox stays nearly empty.", dot: C.deep },
    ],
  },
  {
    id: "week",
    footage: "week",
    kicker: "Week",
    title: "The week at a glance.",
    shots: [
      { at: 0.4, frame: { x: 44, y: 140, width: 760, height: 320 }, caption: "Hours per day, by kind of time.", dot: C.deep },
      { at: 4.6, frame: { x: 44, y: 60, width: 1352, height: 420 }, zoom: 1.15, caption: "When you focus, hour by hour.", sub: "Projects, goals and distractions below.", dot: C.deep },
      { at: "export", frame: { x: 900, y: 40, width: 540, height: 330 }, zoom: 1.9, caption: "Export the week.", sub: "A report, CSV, JSON, or hours by project to bill.", dot: C.ink },
    ],
  },
  {
    id: "trends",
    footage: "trends",
    kicker: "Trends",
    title: "A month of deep work.",
    shots: [
      { at: 0.2, frame: { x: 44, y: 150, width: 1352, height: 120 }, zoom: 1.25, caption: "The month against the last.", dot: C.ink },
      { at: 1.6, frame: { x: 44, y: 285, width: 580, height: 345 }, caption: "A calendar shaded by deep time.", sub: "Hover a day for its score.", dot: C.deep },
      { at: 3.9, frame: { x: 643, y: 285, width: 753, height: 540 }, caption: "Deep time and score, day by day.", sub: "The dashed line is your mean.", dot: C.deep },
      { at: 5.6, frame: "rest" },
      { at: 7.1, frame: { x: 44, y: 420, width: 1352, height: 420 }, zoom: 1.15, caption: "Each category against last month.", sub: "Then your best hours of the day.", dot: C.ink },
    ],
  },
  {
    id: "library",
    footage: "library",
    kicker: "Library",
    title: "Your names, kept tidy.",
    shots: [
      { at: 0.3, frame: { x: 20, y: 60, width: 1400, height: 520 }, zoom: 1.12, caption: "Every project, category and goal.", sub: "Rates, time in the last 30 days, last seen.", dot: C.ink },
      { at: "hint", frame: { x: 44, y: 150, width: 1352, height: 160 }, zoom: 1.3, caption: "Look-alike names, caught.", sub: "Merge North-wind into Northwind in one click.", dot: C.distract },
      { at: "row", frame: { x: 20, y: 60, width: 1400, height: 520 }, zoom: 1.12, caption: "Click a row to change it.", dot: C.shallow },
      { at: "sheet", frame: { x: 1040, y: 0, width: 400, height: 460 }, zoom: 1.75, caption: "Rename, re-file, set a rate.", sub: "Rules, goals and past entries follow the name.", dot: C.shallow },
      { at: "toast", frame: { x: 20, y: 60, width: 1400, height: 520 }, zoom: 1.12, caption: "Saved, and undoable.", dot: C.ink },
    ],
  },
  {
    id: "palette",
    footage: "palette",
    kicker: "⌘K",
    title: "Anywhere in two keys.",
    shots: [
      { at: "palette", frame: "palette", zoom: 1.6, caption: "Jump to a view…", dot: C.ink },
      { at: 4.6, frame: { x: 440, y: 150, width: 560, height: 220 }, zoom: 1.6, caption: "…or straight to a date.", dot: C.ink },
      { at: 7.6, frame: "rest", caption: "Any day, as it was.", dot: C.deep },
    ],
  },
];
