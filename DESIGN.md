---
# gstack: design-md-format=spec
name: tock
description: An instrument panel for your working day. Dark first, one typeface, numbers that line up, colour only for the kind of time.
colors:
  light-bg: "#ffffff"
  light-side: "#f7f7f8"
  light-sunk: "#f1f1f3"
  light-line: "#ececec"
  light-line-strong: "#d8d8d8"
  light-ink: "#1b1f24"
  light-text: "#2b3038"
  light-muted: "#5b636e"
  light-faint: "#6b7280"
  dark-bg: "#0f1011"
  dark-side: "#16171a"
  dark-sunk: "#1d2024"
  dark-line: "#26272b"
  dark-line-strong: "#33373d"
  dark-ink: "#e6e6e8"
  dark-text: "#d5d8dd"
  dark-muted: "#8b8d93"
  dark-faint: "#5d5f66"
  focus: "#2f6fdb"
  good-light: "#2e7d4f"
  good-dark: "#6cc3a3"
  bad-light: "#b3423f"
  bad-dark: "#d98078"
  deep-light: "#32857a"
  deep-dark: "#4fa89a"
  shallow-light: "#4c6ba9"
  shallow-dark: "#6f86b5"
  browse-light: "#8d589d"
  browse-dark: "#a07fb0"
  neutral-light: "#7e828b"
  neutral-dark: "#7d8088"
  distract-light: "#c64539"
  distract-dark: "#c9625a"
  curtain: "#0d0b09"
  curtain-track: "#2b241b"
  curtain-text: "#f1e4cf"
  curtain-muted: "#8a7d68"
typography:
  title:
    fontFamily: Inter
    fontWeight: 600
    fontSize: 1.25rem
  score:
    fontFamily: Inter
    fontWeight: 600
    fontSize: 3.5rem
    letterSpacing: -0.03em
  body:
    fontFamily: Inter
    fontSize: 0.8125rem
    lineHeight: 1.45
  label:
    fontFamily: Inter
    fontWeight: 500
    fontSize: 0.6875rem
  number:
    fontFamily: Inter
    fontWeight: 500
    fontFeature: tnum
rounded:
  sm: 2px
  md: 6px
  lg: 8px
spacing:
  xs: 4px
  sm: 8px
  md: 16px
  lg: 24px
  xl: 32px
components:
  button-primary:
    backgroundColor: "{colors.light-ink}"
    textColor: "{colors.light-bg}"
    rounded: "{rounded.md}"
  button-secondary:
    borderColor: "{colors.light-line-strong}"
    textColor: "{colors.light-ink}"
    rounded: "{rounded.md}"
  nav-active:
    backgroundColor: "{colors.light-sunk}"
    textColor: "{colors.light-ink}"
---

# tock

## Overview

**North star:** an instrument panel. tock reads your day back to you the way a good gauge cluster does: one big reading, the charts that explain it, the tables underneath. Dense, calm, dark.
**Product context:** a local time tracker for one person on a Mac (and a headless Linux box): a daemon, a menu bar app with a notch, a terminal UI and a dashboard that opens in its own tock window. No accounts, no cloud, nothing loaded from the internet.
**References:** variant A "Instrument panel" (`~/.gstack/projects/tock/designs/dashboard-20261105/variant-A.html`), Linear (density, hairlines), Timing.app (rows per category on a time axis).
**Key characteristics:**
- One big number: the focus score, with its change against your usual day. Everything else is a chart or a table.
- Charts carry the analysis: a donut of the kinds of time, goal rings, minutes per hour, an hour heat map, a month calendar.
- Time runs left to right on one shared axis; each category gets its own row.
- Colour means a kind of time. Everything else is grey.

## Colors

**Strategy:** near-black neutrals, one light ink, kind colours as data. No accent colour.
**Light or dark:** dark first (screenshots, the native window's first paint); light follows the system or the ⌘K switch and is first-class too.

- Ink is the only "accent": the selected nav item (on a sunk grey), the primary button, the score line.
- Kind colours are data, never decoration: deep (teal), shallow (blue), browsing (mauve), neutral (grey), distracting (brick). Unlabelled is faint grey; away is hatching. The same colours on every surface: dashboard, TUI, menu strip, notch.
- Good and bad are for changes against the usual day, and follow meaning, not direction: less distraction is good.
- Text: ink for names and numbers, muted for secondary, faint for axes and captions.
- The break screen keeps its fixed curtain colours (always dark).

## Typography

- Inter for everything, with tabular figures (`tnum`) and `cv11`, so numbers line up without a mono face. 13px body, 11px labels in faint small caps with 0.06em tracking, 20px semibold page titles, 56px semibold score.
- Durations read 2h24 in cards and charts, 2:24 (h:mm) in tables.
- Inter is bundled in `internal/web/static/fonts` (OFL, latin subset). The menu bar, notch and break screen use the system fonts; the TUI uses the terminal's.

## Layout

- Dashboard: a 44px top bar (logo, nav, date stepper, ⌘K), a 34px status strip (recording, rules, running session), then a content column up to 1400px.
- In the tock window the top bar is the title bar: it drags the window and leaves room for the traffic lights.
- Today, in order: timeline; score card, minutes per hour, kinds donut; goals, categories, apps; work blocks.
- Week: hours per day and kinds donut; score line and focus heat map; projects donut, goals, distractions; categories and apps.
- Trends: month stat cards; month calendar beside deep-time area and score line; categories against last month; best hours.
- Panels sit in grids with 20px gaps and 20px padding. Below 1024px grids stack.

## Elevation & Depth

Flat. Panels are surface-coloured with a hairline border; no shadows except popovers, tooltips, the sheet and toasts.

## Shapes

2px on chart bars and timeline bars, 3px on heat cells, 6px on buttons and inputs, 8px on panels and the sheet. Donuts and rings use round caps.

## Components

Built from shadcn/ui (Radix) and Recharts in `web/`; the build is committed to `internal/web/static`.
- Score card: label, the score at 56px, change vs usual, then four stats each with its change.
- Kind donut: 66% inner radius, total in the middle, a legend with time and share.
- Goal rings: one ring per goal (at most four), inner first; at-most goals turn brick once passed.
- Category timeline: an hour axis on top, one lane per category plus Away, bars in the kind colour, a 1px ink line for now. Hover shows app, title, time and label; click opens the label sheet.
- Heat map: weekday x hour, deep teal by mean focus minutes. Month calendar: weeks as rows, days shaded by deep time, days with nothing recorded hatched.
- Tables: faint 11px header, hairline rows, right-aligned tabular time and share, a bold totals row.
- Label sheet: a side sheet with a command search over categories; Return labels and learns a rule.
- ⌘K palette: views, dates, theme.
- Toasts: every change has an Undo.

## Do's and Don'ts

- Do give every number a unit and, where it exists, a comparison.
- Do keep numbers in tabular figures and right-aligned in tables.
- Do use the same kind colours in the dashboard, TUI, menu strip and notch.
- Do use rings and donuts for shares and goals; bars for time over time.
- Don't add a second big number, a gauge or a decorative gradient (a chart's area fill is fine).
- Don't put an icon on every row.
- Don't use colour for anything that is not a kind of time or a good/bad change.
- Don't load anything from the internet, fonts included.

## Motion

- Minimal: 120ms hover, 160ms view change, no counting numbers, no growing bars (charts draw without animation).
- The label sheet keeps its spring; reduced motion turns everything off.
- The notch panel is the exception: it grows out of the hardware, so its
  shape and content move on springs that start from where they are. Moments
  tock opens: response 0.46 s, damping 0.78. A click opens: 0.40 s, 0.86.
  Hover: 0.32 s, 1.0. Closing: 0.36 s, 1.0, back up into the notch. A
  resting pointer grows the closed notch by 18 × 3 pt, never more. Content fades in behind the shape.
  Numbers still never count and bars never grow. Reduced motion: the shape
  jumps and content cross-fades in 150 ms.
- The notch is always black, like the hardware, with the dark set of kind
  colours and system fonts, as the menu bar uses.
- A display without a notch gets one drawn: the width of the built-in notch
  (185 pt without one), the height of its menu bar, the same fillets. Face
  text keeps to the edges of the shape and never sits under the camera.

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-11-05 | Initial design system: paper/ink neutrals, brass accent, Instrument Serif + Source Sans 3 + JetBrains Mono | v5 Phase 5d. Unified the web page, TUI and menu with the break screen |
| 2026-11-05 | Replaced by the Ledger system: IBM Plex Sans + Mono, white and grey, ink as the only accent, sidebar, per-category timeline, tables with totals, comparisons with the usual day | The owner picked the Ledger direction from four mockups; the serif-and-brass look read as made by AI |
| 2026-11-05 | A drawn notch on displays without one | Owner asked for the island on every display, as notch apps do; one shape everywhere instead of a pill that looks like another product |
| 2026-11-05 | The notch panel moves on springs | Owner approved the notch prototype; a surface that grows out of hardware reads as physical only with springs. Kept to the notch |
| 2026-11-05 | Dashboard moves to variant A "Instrument panel": React + shadcn/ui + Recharts, Inter, dark first, a big score, donuts and rings, in a native tock window; kind colours change on every surface | Owner: "I don't like the GUI at all", wants components, many charts and a circular one, and an app feel. Supersedes the Ledger rules on hero numbers and rings |
